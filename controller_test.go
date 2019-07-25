package main

import (
	"reflect"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

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
	localActions  []core.Action
	remoteActions []core.Action

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

func (f *fixture) run(fooName string) {
	f.runController(fooName, false)
}

func (f *fixture) runController(fooName string, expectError bool) {
	c, sI, nI := f.newController()

	stopCh := make(chan struct{})
	defer close(stopCh)
	sI.Start(stopCh)
	nI.Start(stopCh)

	err := c.syncHandler(fooName)
	if !expectError && err != nil {
		f.t.Errorf("error syncing foo: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing foo, got nil")
	}

	localActions := filterInformerActions(f.localClient.Actions())
	for i, action := range localActions {
		if len(f.localActions) < i+1 {
			f.t.Errorf("%d unexpected localActions: %+v", len(localActions)-len(f.localActions), localActions[i:])
			break
		}

		expectedAction := f.localActions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.localActions) > len(localActions) {
		f.t.Errorf("%d additional expected localActions:%+v", len(f.localActions)-len(localActions), f.localActions[len(localActions):])
	}

	remoteActions := filterInformerActions(f.remoteClient.Actions())
	for i, action := range remoteActions {
		if len(f.remoteActions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(remoteActions)-len(f.remoteActions), remoteActions[i:])
			break
		}

		expectedAction := f.remoteActions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.remoteActions) > len(remoteActions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.remoteActions)-len(remoteActions), f.remoteActions[len(remoteActions):])
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
	f.localActions = append(
		f.localActions,
		core.NewCreateAction(schema.GroupVersionResource{Resource: "endpoints"}, e.Namespace, e),
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

func TestCreatesEndpoint(t *testing.T) {
	namespace := "foo-namespace"
	name := "foo-name"

	node := &v1.Node{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "foo-node",
		},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{
					Type:    v1.NodeInternalIP,
					Address: "10.11.12.13",
				},
			},
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
				{
					Type:   v1.NodeNetworkUnavailable,
					Status: v1.ConditionFalse,
				},
			},
		},
	}

	service := &v1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    serviceLabel,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name: "foo-port",
					Port: 12345,
				},
			},
		},
	}

	expEndpoint := &v1.Endpoints{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    serviceLabel,
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{
						IP: "10.11.12.13",
					},
				},
				Ports: []v1.EndpointPort{
					{
						Port: 12345,
						Name: "foo-port",
					},
				},
			},
		},
	}

	f := newFixture(t)

	f.serviceLister = append(f.serviceLister, service)
	f.localObjects = append(f.localObjects, service)
	f.nodeLister = append(f.nodeLister, node)
	f.remoteObjects = append(f.remoteObjects, node)

	f.expectCreateEndpointAction(expEndpoint)

	f.run(getKey(service, t))
}
