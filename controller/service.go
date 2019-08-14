package controller

import (
	"barrelman/metrics"
	"barrelman/utils"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

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

	// queue will queue all services that need to be need to create dummy's for (in local)
	queue workqueue.RateLimitingInterface
}

func NewServiceController(
	localClient, remoteClient kubernetes.Interface,
	remoteInformer coreinformers.ServiceInformer) *ServiceController {

	c := &ServiceController{
		localClient:  localClient,
		remoteClient: remoteClient,
		queue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Services"),
	}

	c.remoteServiceLister = remoteInformer.Lister()
	c.remoteSynced = remoteInformer.Informer().HasSynced

	// Enqueue services with type: NodePort that are not ignored by annotation
	remoteInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			service := obj.(*v1.Service)
			if !utils.ResponsibleForService(service) {
				return
			}
			klog.Infof("ADD remote service %s/%s", service.GetNamespace(), service.GetName())
			c.enqueueService(obj)
		},
		UpdateFunc: func(old, cur interface{}) {
			newService := cur.(*v1.Service)
			oldService := old.(*v1.Service)
			if newService.ResourceVersion == oldService.ResourceVersion {
				return
			}
			if !utils.ResponsibleForService(newService) && !utils.ResponsibleForService(oldService) {
				return
			}
			klog.Infof("UPDATE remote service %s/%s", newService.GetNamespace(), newService.GetName())
			c.enqueueService(cur)
		},
		DeleteFunc: func(obj interface{}) {
			service := obj.(*v1.Service)
			if !utils.ResponsibleForService(service) {
				return
			}
			klog.Infof("DELETE remote Service %s/%s", service.GetNamespace(), service.GetName())
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
		if err := c.syncHandler(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.queue.AddRateLimited(key)
			metrics.EndpointUpdateErrors.Inc()
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.queue.Forget(obj)
		metrics.EndpointUpdates.Inc()
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
func (c *ServiceController) syncHandler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
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
		* Check if service exists in local, do nothing if it is

		delete:
		* Delete service from local if barrelman created it (FIXME: how to identify?)

		update:
		* Update local service with new NodePort
	*/
	action := ActionTypeAdd

	// Get the service resource from lister
	service, err := c.remoteServiceLister.Services(namespace).Get(name)
	// FIXME The resource does no longer exist, delete local service
	if errors.IsNotFound(err) {
		action = ActionTypeDelete
	}
	if err != nil {
		// Error fetching remote object
		return err
	}

	// Or the resource may have changed and we're no longer responsible for it
	if !utils.ResponsibleForService(service) {
		action = ActionTypeDelete
	}

	_ = service
	switch action {
	case ActionTypeAdd:
	case ActionTypeUpdate:
	case ActionTypeDelete:
		klog.Infoln(action)
	}

	return nil
}

// enqueueService adds a service (key) to the queue
func (c *ServiceController) enqueueService(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	c.queue.Add(key)
}
