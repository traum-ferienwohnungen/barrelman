package controller

import (
	"barrelman/utils"
	"reflect"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/intstr"

	v1 "k8s.io/api/core/v1"

	"github.com/Pallinder/go-randomdata"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

const (
	serviceName      = "foo-name"
	serviceNamespace = "foo-namespace"
	portName         = "foo-port"
	portNum          = 12345
	portNodePort     = 54321
)

/*
Every test involving slices of items (like multiple ports, multiple hosts etc.) are super hard to test in a generic
way as there is no proper way to DeepEqal structs while ignoring the order of elements in slices.
*/
var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

type fixture struct {
	t *testing.T

	localClient  *k8sfake.Clientset
	remoteClient *k8sfake.Clientset

	// Objects to put in the stores
	serviceLister []*v1.Service
	nodeLister    []*v1.Node

	// Actions expected to happen on the client(s).
	localExpectedActions  []core.Action
	remoteExpectedActions []core.Action

	// Objects from here will be preloaded into informers
	localObjects  []runtime.Object
	remoteObjects []runtime.Object
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	f.localObjects = []runtime.Object{}
	f.remoteObjects = []runtime.Object{}
	return f
}

func (f *fixture) newController() (*NodeEndpointController, kubeinformers.SharedInformerFactory, kubeinformers.SharedInformerFactory) {
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

func (f *fixture) run(serviceName string) {
	f.runController(serviceName, false)
}

func (f *fixture) runControllerTestQueue(numExpectedLocalActions, numExpectedRemoteActions int) {
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
	numLocalActions := len(filterInformerActions(f.localClient.Actions()))
	if numExpectedLocalActions != numLocalActions {
		f.t.Errorf("expeced %d local actions, got %d", numExpectedLocalActions, numLocalActions)
	}
	numRemoteActions := len(filterInformerActions(f.remoteClient.Actions()))
	if numExpectedRemoteActions != numRemoteActions {
		f.t.Errorf("expeced %d remote actions, got %d", numExpectedLocalActions, numLocalActions)
	}
}

func (f *fixture) runController(serviceName string, expectError bool) {
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

func (f *fixture) checkActions() {
	localActions := filterInformerActions(f.localClient.Actions())
	for i, action := range localActions {
		if len(f.localExpectedActions) < i+1 {
			f.t.Errorf("%d unexpected localExpectedActions: %+v", len(localActions)-len(f.localExpectedActions), localActions[i:])
			break
		}

		expectedAction := f.localExpectedActions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.localExpectedActions) > len(localActions) {
		f.t.Errorf("%d additional expected localExpectedActions:%+v", len(f.localExpectedActions)-len(localActions), f.localExpectedActions[len(localActions):])
	}

	remoteActions := filterInformerActions(f.remoteClient.Actions())
	for i, action := range remoteActions {
		if len(f.remoteExpectedActions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(remoteActions)-len(f.remoteExpectedActions), remoteActions[i:])
			break
		}

		expectedAction := f.remoteExpectedActions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.remoteExpectedActions) > len(remoteActions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.remoteExpectedActions)-len(remoteActions), f.remoteExpectedActions[len(remoteActions):])
	}
}

// checkAction verifies that expected and actual actions are equal and both have
// same attached resources
func checkAction(expected, actual core.Action, t *testing.T) {
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource) && actual.GetSubresource() == expected.GetSubresource()) {
		t.Errorf("Expected\n\t%#v\ngot\n\t%#v", expected, actual)
		return
	}

	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		t.Errorf("Action has wrong type. Expected: %t. Got: %t", expected, actual)
		return
	}

	switch a := actual.(type) {
	case core.CreateAction:
		e, _ := expected.(core.CreateAction)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.UpdateAction:
		e, _ := expected.(core.UpdateAction)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.PatchAction:
		e, _ := expected.(core.PatchAction)
		expPatch := e.GetPatch()
		patch := a.GetPatch()

		if !reflect.DeepEqual(expPatch, patch) {
			t.Errorf("Action %s %s has wrong patch\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expPatch, patch))
		}
	}
}

// filterInformerActions filters list and watch actions for testing resources.
// Since get, list and watch don't change resource state we can filter it to
// lower noise level in our tests.
func filterInformerActions(actions []core.Action) []core.Action {
	ret := []core.Action{}
	for _, action := range actions {
		if action.Matches("get", "endpoints") ||
			action.Matches("list", "nodes") ||
			action.Matches("watch", "nodes") ||
			action.Matches("list", "services") ||
			action.Matches("watch", "services") {
			continue
		}
		ret = append(ret, action)
	}

	return ret
}

func (f *fixture) expectCreateEndpointAction(e *v1.Endpoints) {
	f.localExpectedActions = append(
		f.localExpectedActions,
		core.NewCreateAction(schema.GroupVersionResource{Resource: "endpoints"}, e.Namespace, e),
	)
}

func (f *fixture) expectUpdateEndpointAction(e *v1.Endpoints) {
	f.localExpectedActions = append(
		f.localExpectedActions,
		core.NewUpdateAction(schema.GroupVersionResource{Resource: "endpoints"}, e.Namespace, e),
	)
}

func getKey(foo *v1.Service, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(foo)
	if err != nil {
		t.Errorf("Unexpected error getting key for service %v: %v", foo.Name, err)
		return ""
	}
	return key
}

func newNode(internalIP string, ready bool) *v1.Node {
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

func newService() *v1.Service {
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

func newEndpoint(nodeIPs []string) *v1.Endpoints {
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
	f := newFixture(t)

	nodeIP := randomdata.IpV4Address()
	node := newNode(nodeIP, true)
	f.nodeLister = append(f.nodeLister, node)
	f.remoteObjects = append(f.remoteObjects, node)

	brokenNode := newNode(randomdata.IpV4Address(), false)
	f.nodeLister = append(f.nodeLister, brokenNode)
	f.remoteObjects = append(f.remoteObjects, brokenNode)

	service := newService()
	expEndpoint := newEndpoint([]string{nodeIP})
	f.serviceLister = append(f.serviceLister, service)
	f.localObjects = append(f.localObjects, service)

	f.expectCreateEndpointAction(expEndpoint)

	f.run(getKey(service, t))
}

func TestAddNewNode(t *testing.T) {
	f := newFixture(t)

	nodeIP := randomdata.IpV4Address()
	node := newNode(nodeIP, true)
	f.nodeLister = append(f.nodeLister, node)
	f.remoteObjects = append(f.remoteObjects, node)

	brokenNode := newNode(randomdata.IpV4Address(), true)
	brokenNode.Status.Conditions[1] = v1.NodeCondition{
		Type:   v1.NodeNetworkUnavailable,
		Status: v1.ConditionTrue,
	}
	f.nodeLister = append(f.nodeLister, brokenNode)
	f.remoteObjects = append(f.remoteObjects, brokenNode)

	service := newService()
	f.serviceLister = append(f.serviceLister, service)
	f.localObjects = append(f.localObjects, service)

	// Cluster contains an endpoint with no node IP
	endpoint := newEndpoint([]string{})
	f.localObjects = append(f.localObjects, endpoint)

	expEndpoint := newEndpoint([]string{nodeIP})
	f.expectUpdateEndpointAction(expEndpoint)

	f.run(getKey(service, t))
}

func TestBunchOfServices(t *testing.T) {
	f := newFixture(t)

	nodeIP := randomdata.IpV4Address()
	node := newNode(nodeIP, true)
	f.nodeLister = append(f.nodeLister, node)
	f.remoteObjects = append(f.remoteObjects, node)

	service := newService()
	f.serviceLister = append(f.serviceLister, service)
	f.localObjects = append(f.localObjects, service)
	service2 := newService()
	service2.Name += "2"
	f.serviceLister = append(f.serviceLister, service2)
	f.localObjects = append(f.localObjects, service2)

	expEndpoint := newEndpoint([]string{nodeIP})
	f.expectCreateEndpointAction(expEndpoint)
	expEndpoint2 := newEndpoint([]string{nodeIP})
	expEndpoint2.Name += "2"
	f.expectCreateEndpointAction(expEndpoint2)

	f.runControllerTestQueue(2, 0)
}
