package controller

import (
	"barrelman/metrics"
	"barrelman/utils"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/intstr"

	"k8s.io/apimachinery/pkg/api/errors"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

type ActionType string

const (
	ActionTypeNone   = ""
	ActionTypeAdd    = "Add"
	ActionTypeDelete = "Delete"
	ActionTypeUpdate = "Update"
)

type ServiceController struct {
	// localClient is the k8s Clientset for the local cluster (where we create/delete services)
	// remoteClient is the k8s Clientset fot the remote cluster (which we watch for service changes)
	localClient, remoteClient kubernetes.Interface

	// Informer and Indexer for services and their sync state
	remoteServiceLister corelisters.ServiceLister
	remoteSynced        cache.InformerSynced
	localServiceLister  corelisters.ServiceLister
	localSynced         cache.InformerSynced

	// queue will queue all services that need to be need to create dummy's for (in local)
	queue workqueue.RateLimitingInterface

	// Type of the local services to create, defaults to ClusterIP
	localServiceType v1.ServiceType
}

func NewServiceController(
	localClient, remoteClient kubernetes.Interface,
	remoteInformer coreinformers.ServiceInformer, localInformer coreinformers.ServiceInformer,
	createNodePortSvc bool) *ServiceController {

	c := &ServiceController{
		localClient:  localClient,
		remoteClient: remoteClient,
		queue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Services"),
	}

	c.remoteServiceLister = remoteInformer.Lister()
	c.remoteSynced = remoteInformer.Informer().HasSynced

	// localServiceType defaults to ClusterIP
	c.localServiceType = v1.ServiceTypeClusterIP
	if createNodePortSvc {
		c.localServiceType = v1.ServiceTypeNodePort
	}

	// Enqueue services
	// Check for labels, annotations and service type via utils.ResponsibleFor to reduce noise in queue
	remoteInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			service := obj.(*v1.Service)
			if !utils.ResponsibleForRemoteService(service) {
				return
			}

			klog.V(3).Infof("ADD remote service %s/%s", service.GetNamespace(), service.GetName())
			c.enqueueService(obj)
		},
		UpdateFunc: func(old, cur interface{}) {
			newService := cur.(*v1.Service)
			oldService := old.(*v1.Service)
			if newService.ResourceVersion == oldService.ResourceVersion {
				// This is the same object, e.g. resync
				return
			}
			if !utils.ResponsibleForRemoteService(newService) && !utils.ResponsibleForRemoteService(oldService) {
				return
			}
			klog.V(3).Infof("UPDATE remote service %s/%s", newService.GetNamespace(), newService.GetName())
			c.enqueueService(cur)
		},
		DeleteFunc: func(obj interface{}) {
			service := obj.(*v1.Service)
			if !utils.ResponsibleForRemoteService(service) {
				return
			}
			klog.V(3).Infof("DELETE remote Service %s/%s", service.GetNamespace(), service.GetName())
			c.enqueueService(obj)
		},
	})

	c.localServiceLister = localInformer.Lister()
	c.localSynced = localInformer.Informer().HasSynced

	// Enqueue services that have been deleted in local
	// This is the case when we've already deployed to VPC cluster and deleting the helm release in legacy cluster
	// afterwards. In that case, we want barrelman to create a dummy service in local-cluster immediately.
	localInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			service := obj.(*v1.Service)
			klog.V(3).Infof("DELETE local Service %s/%s", service.GetNamespace(), service.GetName())
			c.enqueueService(obj)
		},
	})

	return c
}

func (c *ServiceController) Run(workers int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Infof("Starting ServiceController")

	// and wait for their caches to warm up
	klog.Info("Waiting for informer caches to warm up")
	if !cache.WaitForCacheSync(stopCh, c.remoteSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Infof("Starting %d workers", workers)
	for i := 0; i < workers; i++ {
		go wait.Until(c.worker, time.Second, stopCh)
	}

	<-stopCh
	klog.Infof("Shutting down workers")
	return nil
}

func (c *ServiceController) worker() {
	for c.processNextItem() {
	}
}

func (c *ServiceController) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := c.queue.Get()
	if quit {
		return false
	}

	// We wrap this block in a func so we can defer c.queue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the queue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the queue and attempted again after a back-off
		// period.
		defer c.queue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the queue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// queue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// queue.
		if key, ok = obj.(string); !ok {
			// As the item in the queue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.queue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Foo resource to be synced.
		action, err := c.syncHandler(key)
		mA := string(action)
		if action == ActionTypeNone {
			mA = "None"
		}
		if err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.queue.AddRateLimited(key)
			metrics.ServiceUpdateErrors.WithLabelValues(mA).Inc()
			metrics.ObjectsQueued.WithLabelValues("ServiceController", "true").Inc()
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.queue.Forget(obj)
		metrics.ServiceUpdates.WithLabelValues(mA).Inc()
		return nil
	}(key)

	if err != nil {
		runtime.HandleError(err)
		return true
	}
	return true
}

// syncHandler fetches the object from indexer and does cache warmup
// In case an error happened, it has to simply return the error.
// The retry logic should not be part of the business logic.
func (c *ServiceController) syncHandler(key string) (ActionType, error) {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return ActionTypeNone, nil
	}

	/*
			Services are queued so things may have changed so we need to figure out
			the desired action now.
			There are the following options:
			* Service is new -> create
			* Service is gone -> delete
			* NodePort has changed -> update
			* Ignore annotation has been added -> delete
			* Ignore annotation has been removed -> create
			* Service type has changes to NodePort -> create
			* Service type has changed from NodePort -> delete

		create:
		* Check if remoteService exists in local, do nothing if it is

		delete:
		* Delete remoteService from local if barrelman created it (utils.ResourceLabel)

		update:
		* Update local remoteService with new NodePort
	*/

	// Get remote and local service objects
	getFunc := func() (*v1.Service, error) {
		return c.remoteServiceLister.Services(namespace).Get(name)
	}
	remoteSvc, remoteExists, err := utils.GetService(getFunc)
	if err != nil {
		return ActionTypeNone, err
	}

	getFunc = func() (*v1.Service, error) {
		return c.localClient.CoreV1().Services(namespace).Get(name, metaV1.GetOptions{})
	}
	localSvc, localExists, err := utils.GetService(getFunc)
	if err != nil {
		return ActionTypeNone, err
	}

	// Check what action we need to take on local cluster
	action := getLocalAction(remoteExists, remoteSvc, localExists, localSvc)

	switch action {
	case ActionTypeAdd:
		// Check if namespace exist
		_, err := c.localClient.CoreV1().Namespaces().Get(namespace, metaV1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				return action, err
			}

			// If namespace does not exist (in local), create it
			klog.Infof("performing \"%s\" action for namespace %s", action, namespace)
			_, nsErr := c.localClient.CoreV1().Namespaces().Create(&v1.Namespace{
				ObjectMeta: metaV1.ObjectMeta{
					Name: namespace,
				},
			})
			if nsErr != nil {
				klog.Errorf("Failed creating namespace '%s' in local cluster", namespace)
				return action, nsErr
			}
		}
		// Build dummy service ports
		dummyPorts := c.getDummyServicePorts(remoteSvc)
		// Create dummy service
		klog.Infof("performing \"%s\" action for service %s/%s", action, namespace, name)
		_, err = c.localClient.CoreV1().Services(namespace).Create(&v1.Service{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    utils.ResourceLabel,
			},
			Spec: v1.ServiceSpec{
				Ports: dummyPorts,
				Type:  c.localServiceType,
			},
		})
		return action, err
	case ActionTypeUpdate:
		dummyPorts := c.getDummyServicePorts(remoteSvc)
		if utils.ServicePortsEqual(localSvc.Spec.Ports, dummyPorts) {
			return ActionTypeNone, nil
		}
		// Update localSvc with new port(s)
		localSvc.Spec.Ports = dummyPorts
		// When barrelman is restarted with -nodeportsvc flag, localSvc may need to change type
		localSvc.Spec.Type = c.localServiceType
		// NodeEndpointController will pick this up and update endpoints
		klog.Infof("performing \"%s\" action for service %s/%s", action, namespace, name)
		_, err := c.localClient.CoreV1().Services(namespace).Update(localSvc)
		return action, err
	case ActionTypeDelete:
		// Delete localSvc
		klog.Infof("performing \"%s\" action for service %s/%s", action, namespace, name)
		return action, c.localClient.CoreV1().Services(namespace).Delete(name, &metaV1.DeleteOptions{})
	case ActionTypeNone:
		return action, nil
	}

	return ActionTypeNone, fmt.Errorf("something wired happened in service syncHandler")
}

// getDummyServicePorts created a new slice of ServicePort to be used for the local dummy service
// For each port, the remote service NodePort must be the dummy service target port (so endpoints will
// point to remote NodePort)
func (c *ServiceController) getDummyServicePorts(remoteSvc *v1.Service) []v1.ServicePort {
	dummyPorts := make([]v1.ServicePort, len(remoteSvc.Spec.Ports))
	for idx, port := range remoteSvc.Spec.Ports {
		// Ensure we don't modify the input
		dummyPorts[idx] = *port.DeepCopy()
		dummyPorts[idx].TargetPort = intstr.FromInt(int(port.NodePort))
		if c.localServiceType != v1.ServiceTypeNodePort {
			// Unset NodePort
			dummyPorts[idx].NodePort = 0
		}
	}
	return dummyPorts
}

// getLocalAction returns the type of action (ActionType) to take on local service
func getLocalAction(remoteExists bool, remoteSvc *v1.Service, localExists bool, localSvc *v1.Service) ActionType {
	if remoteExists && utils.ResponsibleForRemoteService(remoteSvc) {
		klog.V(4).Infof("remote: %s/%s I'm responsible", remoteSvc.GetNamespace(), remoteSvc.GetName())

		if localExists {
			if !utils.OwnerOfService(localSvc) {
				klog.V(4).Infof("local: %s/%s I don't own this service, SKIP", localSvc.GetNamespace(), localSvc.GetName())
				return ActionTypeNone
			}

			if !utils.ResponsibleForService(localSvc) {
				klog.V(4).Infof("local: %s/%s not responsible for service, SKIP", localSvc.GetNamespace(), localSvc.GetName())
				return ActionTypeNone
			}

			klog.V(4).Infof("remote,local: %s/%s both exist, UPDATE", localSvc.GetNamespace(), localSvc.GetName())
			return ActionTypeUpdate
		} else {
			klog.V(4).Infof("local: %s/%s does not exist, ADD", remoteSvc.GetNamespace(), remoteSvc.GetName())
			return ActionTypeAdd
		}
	}

	if !remoteExists || !utils.ResponsibleForRemoteService(remoteSvc) {
		// It's not completely sure that remoteSvc is not nil, so we can't log namespace and name
		klog.V(4).Infoln("remote: not responsible")

		if localExists {
			if !utils.OwnerOfService(localSvc) {
				klog.V(4).Infof("local: %s/%s I don't own this service, SKIP", localSvc.GetNamespace(), localSvc.GetName())
				return ActionTypeNone
			}

			if !utils.ResponsibleForService(localSvc) {
				klog.V(4).Infof("local: %s/%s exists but not responsible, SKIP", localSvc.GetNamespace(), localSvc.GetName())
				return ActionTypeNone
			}
			klog.V(4).Infof("local: %s/%s does exist, DELETE", localSvc.GetNamespace(), localSvc.GetName())
			return ActionTypeDelete
		}
	}

	return ActionTypeNone
}

// enqueueService adds a service (key) to the queue
func (c *ServiceController) enqueueService(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	c.queue.Add(key)
	metrics.ObjectsQueued.WithLabelValues("ServiceController", "false").Inc()
}
