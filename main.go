package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"barrelman/controller"
	"barrelman/utils"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

var (
	// Command line flags
	addr              = flag.String("listen-address", ":9193", "the address to listen for HTTP requests")
	localKubeConfig   = flag.String("local-kubeconfig", "", "absolute path to the kubeconfig file for the \"local\" cluster (where to maintain endpoints)")
	localContext      = flag.String("local-context", "", "context to use as the \"local\" cluster (where to maintain endpoints)")
	remoteProject     = flag.String("remote-project", "", "Remote clusters project id")
	remoteZone        = flag.String("remote-zone", "europe-west1-c", "Remote clusters zone")
	remoteClusterName = flag.String("remote-cluster-name", "", "Remote clusters name")
	resyncPeriod      = flag.Duration("resync-period", 2*time.Hour, "how often should all nodes be considered \"old\" (and processed again)")
	necWorkers        = flag.Uint("nec-workers", 4, "number of workers for NodeEndpointController")
	scWorkers         = flag.Uint("sc-workers", 2, "number of workers for ServiceController")
	// See init() for "ignore-namespace"
)

func init() {
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(),
			`This tool needs to connect to two clusters calles "local" and "remote".
The remote cluster will be watched for node changes.
On change, service endpoints in local cluster will be modify to always contain a up to date list of node ips.

Local cluster may be defined via 'local-kubeconfig' and 'local-context'.
Remote cluster must be defined via 'remote-project', 'remote-zone' and 'remote-cluster-name'.
The the needed config will be auto generated via a Google service account (GOOGLE_APPLICATION_CREDENTIALS).
`)
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "\nUsage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Var(utils.IgnoredNamespaces, "ignore-namespace",
		"namespace to ignore services in, may be given multiple times. Prefix namespace with a dash to remove it from default")
	klog.InitFlags(nil)
}

func getLocalClientset() *kubernetes.Clientset {
	// creates the kubernetes config for the local cluster
	// if kubeconfig is not given, master url is tried
	// if both are omitted, inCluster config is tried
	var config *rest.Config
	var err error
	if *localKubeConfig == "" {
		klog.Infof("No -local-kubeconfig was specified. Using the inClusterConfig.")
		config, err = rest.InClusterConfig()
		if err != nil {
			klog.Fatal(err)
		}
	} else {
		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{
				ExplicitPath: *localKubeConfig,
			},
			&clientcmd.ConfigOverrides{
				CurrentContext: *localContext,
			}).ClientConfig()
		if err != nil {
			klog.Fatal(err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatal(err)
	}

	return clientset
}

func getRemoteClientset() *kubernetes.Clientset {
	if *remoteProject == "" || *remoteZone == "" || *remoteClusterName == "" {
		klog.Fatalln("You have to specify -remote-project, -remote-zone and -remote-cluster-name")
	}

	clientset, err := utils.NewGKEClientset(*remoteProject, *remoteZone, *remoteClusterName)
	if err != nil {
		klog.Fatal(err)
	}
	return clientset
}

func main() {
	flag.Parse()

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := utils.SetupSignalHandler()

	// create the clientsets
	localClientset := getLocalClientset()
	remoteClientset := getRemoteClientset()

	lservices, err := localClientset.CoreV1().Services("").List(metaV1.ListOptions{
		LabelSelector: utils.ServiceSelector.String(),
	})
	if err != nil {
		klog.Fatal(err)
	}
	klog.Infof("%d services to manage endpoints for in local-cluster\n", len(lservices.Items))
	rnodes, err := remoteClientset.CoreV1().Nodes().List(metaV1.ListOptions{})
	if err != nil {
		klog.Fatal(err)
	}
	klog.Infof("%d nodes in remote-cluster\n", len(rnodes.Items))

	localInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(
		localClientset,
		*resyncPeriod,
		kubeinformers.WithTweakListOptions(func(options *metaV1.ListOptions) {
			options.LabelSelector = utils.ServiceSelector.String()
		}),
	)
	remoteInformerFactory := kubeinformers.NewSharedInformerFactory(remoteClientset, *resyncPeriod)

	nodeEndpointController := controller.NewNodeEndpointController(
		localClientset, remoteClientset,
		localInformerFactory.Core().V1().Services(),
		remoteInformerFactory.Core().V1().Nodes(),
	)

	serviceController := controller.NewServiceController(
		localClientset, remoteClientset,
		remoteInformerFactory.Core().V1().Services(),
	)

	// Ramp up the informer loops
	// They run all registered informer in go routines
	localInformerFactory.Start(stopCh)
	remoteInformerFactory.Start(stopCh)

	// Register http handler for metrics and readiness/liveness probe
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "OK")
	})
	httpServer := &http.Server{Addr: *addr}
	go func() {
		// Launch HTTP server
		_ = httpServer.ListenAndServe()
	}()

	// Launch the controllers
	// This will block 'till stopCh
	func() {
		go func() {
			if err = nodeEndpointController.Run(int(*necWorkers), stopCh); err != nil {
				klog.Fatalf("Error running nodeEndpointController: %s", err.Error())
			}
		}()
		go func() {
			if err = serviceController.Run(int(*scWorkers), stopCh); err != nil {
				klog.Fatalf("Error running serviceController: %s", err.Error())
			}
		}()
		<-stopCh
	}()

	// Gracefully stop HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	if err := httpServer.Shutdown(ctx); err != nil {
		klog.Fatalf("Error stopping HTTP server: %v", err)
	}
	// Make sure context is canceled in any case to make linter happy
	cancel()
}
