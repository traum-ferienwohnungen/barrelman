package controller

import (
	"fmt"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"

	"barrelman/metrics"
	"barrelman/utils"

	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

type NodeEndpointController struct {
	// localClient is the k8s Clientset for the local cluster (where we update service endpoints)
	localClient kubernetes.Interface

	// remoteClient is the k8s Clientset fot the remote cluster (which we watch for node changes)
	remoteClient kubernetes.Interface

	// Informer and Indexer for services and nodes
	serviceLister             corelisters.ServiceLister
	nodeLister                corelisters.NodeLister
	serviceSynced, nodeSynced cache.InformerSynced

	// queue will queue all services whose endpoints may need updates
	queue workqueue.RateLimitingInterface
}

func NewNodeEndpointController(
	localClient, remoteClient kubernetes.Interface,
	serviceInformer coreinformers.ServiceInformer,
	nodeInformer coreinformers.NodeInformer) *NodeEndpointController {

	c := &NodeEndpointController{
		localClient:  localClient,
		remoteClient: remoteClient,
		queue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "NodeEndpoints"),
	}

	c.serviceLister = serviceInformer.Lister()
	c.serviceSynced = serviceInformer.Informer().HasSynced

	// Just queue all service events
	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			service := obj.(*v1.Service)
			klog.V(3).Infof("ADD local service %s/%s", service.GetNamespace(), service.GetName())
			c.enqueueService(obj)
		},
		UpdateFunc: func(old, cur interface{}) {
			newService := cur.(*v1.Service)
			oldService := old.(*v1.Service)
			if newService.ResourceVersion == oldService.ResourceVersion {
				return
			}
			klog.V(3).Infof("UPDATE local service %s/%s", newService.GetNamespace(), newService.GetName())
			c.enqueueService(cur)
		},
		DeleteFunc: func(obj interface{}) {
			service := obj.(*v1.Service)
			klog.V(3).Infof("DELETE local service %s/%s", service.GetNamespace(), service.GetName())
			c.enqueueService(obj)
		},
	})

	c.nodeLister = nodeInformer.Lister()
	c.nodeSynced = nodeInformer.Informer().HasSynced

	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addNode,
		UpdateFunc: c.updateNode,
		DeleteFunc: c.deleteNode,
	})

	return c
}

func (c *NodeEndpointController) Run(workers int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Infof("Starting NodeEndpointController")

	// and wait for their caches to warm up
	klog.Info("Waiting for informer caches to warm up")
	if !cache.WaitForCacheSync(stopCh, c.serviceSynced, c.nodeSynced) {
		return fmt.Errorf("Failed to wait for caches to sync")
	}

	klog.Infof("Starting %d workers", workers)
	for i := 0; i < workers; i++ {
		go wait.Until(c.worker, time.Second, stopCh)
	}

	<-stopCh
	klog.Infof("Shutting down workers")
	return nil
}

func (c *NodeEndpointController) worker() {
	for c.processNextItem() {
	}
}

func (c *NodeEndpointController) processNextItem() bool {
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
			metrics.ObjectsQueued.WithLabelValues("NodeEndpointController", "true").Inc()
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
func (c *NodeEndpointController) syncHandler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the service resource from lister
	service, err := c.serviceLister.Services(namespace).Get(name)
	if err != nil {
		// The resource may no longer exist, in which case we stop processing.
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("service '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	nodes, err := c.nodeLister.List(labels.Everything())
	if err != nil {
		runtime.HandleError(fmt.Errorf("error listing nodes in remote cluster: %#v", err))
	}

	endpoint, err := c.localClient.CoreV1().Endpoints(namespace).Get(name, metaV1.GetOptions{})
	if err != nil {
		// Check if endpoint object (same name as service) exists
		if errors.IsNotFound(err) {
			klog.Infof("Creating new endpoint %s", key)
			endpoint, err = utils.NewEndpoint(service, nodes)
			if err != nil {
				klog.Errorln(err)
				return err
			}

			// Create endpoint
			_, err = c.localClient.CoreV1().Endpoints(namespace).Create(endpoint)
			return err
		}

		// Unknown error, just return that one
		return err
	}

	// Endpoint exists, update it's addresses
	klog.Infof("Updating endpoint for %s", key)
	epSubset, err := utils.EndpointSubset(service, nodes)
	if err != nil {
		return err
	}
	endpoint.Subsets = epSubset
	_, err = c.localClient.CoreV1().Endpoints(namespace).Update(endpoint)
	if err != nil {
		return err
	}

	return nil
}

// enqueueService adds a service (key) to the queue
func (c *NodeEndpointController) enqueueService(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	c.queue.Add(key)
	metrics.ObjectsQueued.WithLabelValues("NodeEndpointController", "false").Inc()
}

// enqueueAllServices add all services to the queue
func (c *NodeEndpointController) enqueueAllServices() {
	// serviceLister is already filtered, so we could use an empty label filter here
	//services, err := c.serviceLister.List(labels.Everything())

	services, err := c.serviceLister.List(utils.ServiceSelector)
	if err != nil {
		klog.Infof("No services to enqueue")
		return
	}
	for _, s := range services {
		c.enqueueService(s)
	}
}

func (c *NodeEndpointController) addNode(obj interface{}) {
	node := obj.(*v1.Node)
	klog.V(3).Infof("ADD for Node %s", node.GetName())
	defer metrics.NodeCount.Inc()

	// Check if node is ready
	if !utils.IsNodeReady(node) {
		klog.Warningf("Node %s is not ready", node.GetName())
		return
	}

	internalIP, err := utils.GetNodeInternalIP(node)
	if err != nil {
		klog.Errorln(err)
		return
	}
	klog.Infof("Node %s, IP: %s", node.GetName(), internalIP)
	c.enqueueAllServices()
}

func (c *NodeEndpointController) updateNode(old, cur interface{}) {
	newNode := cur.(*v1.Node)
	oldNode := old.(*v1.Node)

	if newNode.ResourceVersion == oldNode.ResourceVersion {
		// Resync will send update events for all nodes
		return
	}

	// filter out relevant node changes
	var relevantChange bool
	if utils.IsNodeReady(newNode) != utils.IsNodeReady(oldNode) {
		relevantChange = true
	}

	// We ignore ip not found errors here because we can't do anything about it
	// If one of the states does not have an IP, we trigger a sync
	// If none of the states has an IP, we don't trigger a sync
	newNodeIP, _ := utils.GetNodeInternalIP(newNode)
	oldNodeIP, _ := utils.GetNodeInternalIP(oldNode)
	if newNodeIP != oldNodeIP {
		relevantChange = true
	}

	if !relevantChange {
		return
	}

	klog.V(3).Infof("UPDATE for Node %s", newNode.GetName())
	c.enqueueAllServices()
}

func (c *NodeEndpointController) deleteNode(obj interface{}) {
	node := obj.(*v1.Node)
	klog.V(3).Infof("DELETE for Node %s", node.GetName())
	defer metrics.NodeCount.Dec()

	c.enqueueAllServices()
}
