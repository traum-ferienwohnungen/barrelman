package controller

import (
	"barrelman/utils"
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"

	v1 "k8s.io/api/core/v1"

	"github.com/Pallinder/go-randomdata"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

/*
Every test involving slices of items (like multiple ports, multiple hosts etc.) are super hard to test in a generic
way as there is no proper way to DeepEqal structs while ignoring the order of elements in slices.
*/

type necFixture struct {
	baseFixture

	// Objects to put in the stores
	serviceLister []*v1.Service
	nodeLister    []*v1.Node
}

func newNecFixture(t *testing.T) *necFixture {
	f := &necFixture{
		baseFixture: baseFixture{
			t:             t,
			localObjects:  []runtime.Object{},
			remoteObjects: []runtime.Object{},
			informerFilter: []filterAction{
				{"get", "endpoints"},
				{"list", "nodes"},
				{"watch", "nodes"},
				{"list", "services"},
				{"watch", "services"},
			},
		},
	}
	return f
}

func (f *necFixture) newController() (*NodeEndpointController, kubeinformers.SharedInformerFactory, kubeinformers.SharedInformerFactory) {
	f.localClient = k8sfake.NewSimpleClientset(f.localObjects...)
	f.remoteClient = k8sfake.NewSimpleClientset(f.remoteObjects...)

	serviceInformer := kubeinformers.NewSharedInformerFactory(f.localClient, noResyncPeriodFunc())
	nodeInformer := kubeinformers.NewSharedInformerFactory(f.remoteClient, noResyncPeriodFunc())

	c := NewNodeEndpointController(
		f.localClient,
		f.remoteClient,
		serviceInformer.Core().V1().Services(),
		nodeInformer.Core().V1().Nodes(),
	)

	c.serviceSynced = alwaysReady
	c.nodeSynced = alwaysReady

	// Preload test objects into informers
	for _, s := range f.serviceLister {
		err := serviceInformer.Core().V1().Services().Informer().GetIndexer().Add(s)
		if err != nil {
			f.t.Errorf("Failed to add service: %v", err)
		}
	}

	for _, n := range f.nodeLister {
		err := nodeInformer.Core().V1().Nodes().Informer().GetIndexer().Add(n)
		if err != nil {
			f.t.Errorf("Failed to add node: %v", err)
		}
	}

	return c, serviceInformer, nodeInformer
}

func (f *necFixture) run(serviceName string) {
	f.runController(serviceName, false)
}

func (f *necFixture) runControllerTestQueue(numExpectedLocalActions, numExpectedRemoteActions int) {
	c, sI, nI := f.newController()

	stopCh := make(chan struct{})
	defer close(stopCh)
	sI.Start(stopCh)
	nI.Start(stopCh)

	c.enqueueAllServices()
	items := c.queue.Len()
	for i := 1; i <= items; i++ {
		c.processNextItem()
	}

	// Just test the number of actions here as order is not fixed
	numLocalActions := len(f.filterInformerActions(f.localClient.Actions()))
	if numExpectedLocalActions != numLocalActions {
		f.t.Errorf("expeced %d local actions, got %d", numExpectedLocalActions, numLocalActions)
	}
	numRemoteActions := len(f.filterInformerActions(f.remoteClient.Actions()))
	if numExpectedRemoteActions != numRemoteActions {
		f.t.Errorf("expeced %d remote actions, got %d", numExpectedLocalActions, numLocalActions)
	}
}

func (f *necFixture) runController(serviceName string, expectError bool) {
	c, sI, nI := f.newController()

	stopCh := make(chan struct{})
	defer close(stopCh)
	sI.Start(stopCh)
	nI.Start(stopCh)

	err := c.syncHandler(serviceName)
	if !expectError && err != nil {
		f.t.Errorf("error syncing foo: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing foo, got nil")
	}

	f.checkActions()
}

func (f *necFixture) expectCreateEndpointAction(e *v1.Endpoints) {
	f.localExpectedActions = append(
		f.localExpectedActions,
		core.NewCreateAction(schema.GroupVersionResource{Resource: "endpoints"}, e.Namespace, e),
	)
}

func (f *necFixture) expectUpdateEndpointAction(e *v1.Endpoints) {
	f.localExpectedActions = append(
		f.localExpectedActions,
		core.NewUpdateAction(schema.GroupVersionResource{Resource: "endpoints"}, e.Namespace, e),
	)
}

func necNewNode(internalIP string, ready bool) *v1.Node {
	nodeReady := v1.ConditionFalse
	if ready {
		nodeReady = v1.ConditionTrue
	}

	node := &v1.Node{
		ObjectMeta: metaV1.ObjectMeta{
			Name: randomdata.SillyName(),
		},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{
					Type:    v1.NodeInternalIP,
					Address: internalIP,
				},
			},
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: nodeReady,
				},
				{
					Type:   v1.NodeNetworkUnavailable,
					Status: v1.ConditionFalse,
				},
			},
		},
	}
	return node
}

func necNewService() *v1.Service {
	service := &v1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      serviceName,
			Namespace: serviceNamespace,
			Labels:    utils.ServiceLabel,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       portName,
					Port:       portNum,
					TargetPort: intstr.FromInt(portNodePort),
				},
			},
		},
	}
	return service
}

func necNewEndpoint(nodeIPs []string) *v1.Endpoints {
	var epAddresses []v1.EndpointAddress
	for _, ip := range nodeIPs {
		epAddresses = append(epAddresses, v1.EndpointAddress{IP: ip})
	}
	expEndpoint := &v1.Endpoints{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      serviceName,
			Namespace: serviceNamespace,
			Labels:    utils.ServiceLabel,
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: epAddresses,
				Ports: []v1.EndpointPort{
					{
						Port: portNodePort,
						Name: portName,
					},
				},
			},
		},
	}
	return expEndpoint
}

func TestCreatesEndpoint(t *testing.T) {
	f := newNecFixture(t)

	nodeIP := randomdata.IpV4Address()
	node := necNewNode(nodeIP, true)
	f.nodeLister = append(f.nodeLister, node)
	f.remoteObjects = append(f.remoteObjects, node)

	brokenNode := necNewNode(randomdata.IpV4Address(), false)
	f.nodeLister = append(f.nodeLister, brokenNode)
	f.remoteObjects = append(f.remoteObjects, brokenNode)

	service := necNewService()
	expEndpoint := necNewEndpoint([]string{nodeIP})
	f.serviceLister = append(f.serviceLister, service)
	f.localObjects = append(f.localObjects, service)

	f.expectCreateEndpointAction(expEndpoint)

	f.run(getKey(service, t))
}

func TestAddNewNode(t *testing.T) {
	f := newNecFixture(t)

	nodeIP := randomdata.IpV4Address()
	node := necNewNode(nodeIP, true)
	f.nodeLister = append(f.nodeLister, node)
	f.remoteObjects = append(f.remoteObjects, node)

	brokenNode := necNewNode(randomdata.IpV4Address(), true)
	brokenNode.Status.Conditions[1] = v1.NodeCondition{
		Type:   v1.NodeNetworkUnavailable,
		Status: v1.ConditionTrue,
	}
	f.nodeLister = append(f.nodeLister, brokenNode)
	f.remoteObjects = append(f.remoteObjects, brokenNode)

	service := necNewService()
	f.serviceLister = append(f.serviceLister, service)
	f.localObjects = append(f.localObjects, service)

	// Cluster contains an endpoint with no node IP
	endpoint := necNewEndpoint([]string{})
	f.localObjects = append(f.localObjects, endpoint)

	expEndpoint := necNewEndpoint([]string{nodeIP})
	f.expectUpdateEndpointAction(expEndpoint)

	f.run(getKey(service, t))
}

func TestBunchOfServices(t *testing.T) {
	f := newNecFixture(t)

	nodeIP := randomdata.IpV4Address()
	node := necNewNode(nodeIP, true)
	f.nodeLister = append(f.nodeLister, node)
	f.remoteObjects = append(f.remoteObjects, node)

	service := necNewService()
	f.serviceLister = append(f.serviceLister, service)
	f.localObjects = append(f.localObjects, service)
	service2 := necNewService()
	service2.Name += "2"
	f.serviceLister = append(f.serviceLister, service2)
	f.localObjects = append(f.localObjects, service2)

	expEndpoint := necNewEndpoint([]string{nodeIP})
	f.expectCreateEndpointAction(expEndpoint)
	expEndpoint2 := necNewEndpoint([]string{nodeIP})
	expEndpoint2.Name += "2"
	f.expectCreateEndpointAction(expEndpoint2)

	f.runControllerTestQueue(2, 0)
}
