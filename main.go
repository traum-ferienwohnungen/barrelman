package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

	// Prometheus metrics
	nodesCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "barrelman_current_nodes_count",
		Help: "Count of nodes in watched cluster.",
	})
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `This tool needs to connect to two clusters calles "local" and "remote".
The remote cluster will be watched for node changes.
On change, service endpoints in local cluster will be modify to always contain a up to date list of node ips.

Local cluster may be defined via 'local-kubeconfig' and 'local-context'.
Remote cluster must be defined via 'remote-project', 'remote-zone' and 'remote-cluster-name'.
The the needed config will be auto generated via a Google service account (GOOGLE_APPLICATION_CREDENTIALS).
`)
		fmt.Fprintf(flag.CommandLine.Output(), "\nUsage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}
	klog.InitFlags(nil)
	// Register prometheus metrics
	prometheus.MustRegister(nodesCount)
}

func getLocalClientset() *kubernetes.Clientset {
	// creates the kubernetes config for the local cluster
	// if kubeconfig is not given, master url is tried
	// if both are omitted, inCluster config is tried
	var config *rest.Config
	var err error
	if *localKubeConfig == "" {
		klog.Infof("No -local-kubeconfig was specified.  Using the inClusterConfig.  This might not work.")
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

	clientset, err := NewGKEClientset(*remoteProject, *remoteZone, *remoteClusterName)
	if err != nil {
		klog.Fatal(err)
	}
	return clientset
}

func main() {
	flag.Parse()

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := SetupSignalHandler()

	// create the clientsets
	localClientset := getLocalClientset()
	remoteClientset := getRemoteClientset()

	serviceLabelSelector := labels.Set(map[string]string{"tfw.io/barrelman": "true"}).AsSelector()

	lservices, err := localClientset.CoreV1().Services("").List(metaV1.ListOptions{
		LabelSelector: serviceLabelSelector.String(),
	})
	if err != nil {
		klog.Fatal(err)
	}
	klog.Infof("%d services in local\n", len(lservices.Items))
	rnodes, err := remoteClientset.CoreV1().Nodes().List(metaV1.ListOptions{})
	if err != nil {
		klog.Fatal(err)
	}
	klog.Infof("%d nodes in remote\n", len(rnodes.Items))

	localInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(
		localClientset,
		*resyncPeriod,
		kubeinformers.WithTweakListOptions(func(options *metaV1.ListOptions) {
			options.LabelSelector = serviceLabelSelector.String()
		}),
	)
	remoteInformerFactory := kubeinformers.NewSharedInformerFactory(remoteClientset, *resyncPeriod)

	controller := NewNodeEndpointController(
		localClientset, remoteClientset,
		localInformerFactory.Core().V1().Services(),
		remoteInformerFactory.Core().V1().Nodes(),
	)

	// Ramp up the informer loops
	// They run all registered informer in go routines
	localInformerFactory.Start(stopCh)
	remoteInformerFactory.Start(stopCh)

	var runErr error
	go func() {
		runErr = controller.Run(1, stopCh)
	}()
	if runErr != nil {
		klog.Fatal(err)
	}

	// Register http handler for metrics and readiness/liveness probe
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "OK")
	})
	// Launch HTTP server
	klog.Fatal(http.ListenAndServe(*addr, nil))
}
