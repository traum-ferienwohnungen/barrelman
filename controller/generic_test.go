package controller

import (
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"

	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/util/diff"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

/*
Generic stuff for controller tests
*/

const (
	serviceName      = "foo-name"
	serviceNamespace = "foo-namespace"
	portName         = "foo-port"
	portNum          = 12345
	portNodePort     = 54321
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

type filterAction struct {
	Verb, Resource string
}

type baseFixture struct {
	t *testing.T

	localClient  *k8sfake.Clientset
	remoteClient *k8sfake.Clientset

	// Actions expected to happen on the client(s).
	localExpectedActions  []core.Action
	remoteExpectedActions []core.Action

	// Objects from here will be preloaded into informers
	localObjects  []runtime.Object
	remoteObjects []runtime.Object

	// List of actions to filter out for testing
	informerFilter []filterAction
}

func init() {
	spew.Config.DisablePointerAddresses = true
	spew.Config.DisableMethods = true
}

func (b *baseFixture) filterInformerActions(actions []core.Action) []core.Action {
	var ret []core.Action
	for _, action := range actions {
		skip := false
		for _, fAction := range b.informerFilter {
			if action.Matches(fAction.Verb, fAction.Resource) {
				skip = true
				break
			}
		}
		if !skip {
			ret = append(ret, action)
		}
	}

	return ret
}

func (b *baseFixture) checkActions() {
	localActions := b.filterInformerActions(b.localClient.Actions())
	for i, action := range localActions {
		if len(b.localExpectedActions) < i+1 {
			b.t.Errorf("%d unexpected local actions:\n%s",
				len(localActions)-len(b.localExpectedActions),
				spew.Sdump(localActions[i:]))
			break
		}

		expectedAction := b.localExpectedActions[i]
		checkAction(expectedAction, action, b.t)
	}

	if len(b.localExpectedActions) > len(localActions) {
		b.t.Errorf("%d additional expected local actions:\n%s",
			len(b.localExpectedActions)-len(localActions),
			spew.Sdump(b.localExpectedActions[len(localActions):]))
	}

	remoteActions := b.filterInformerActions(b.remoteClient.Actions())
	for i, action := range remoteActions {
		if len(b.remoteExpectedActions) < i+1 {
			b.t.Errorf("%d unexpected remote actions:\n%s",
				len(remoteActions)-len(b.remoteExpectedActions),
				spew.Sdump(remoteActions[i:]))
			break
		}

		expectedAction := b.remoteExpectedActions[i]
		checkAction(expectedAction, action, b.t)
	}

	if len(b.remoteExpectedActions) > len(remoteActions) {
		b.t.Errorf("%d additional expected remote actions:\n%s",
			len(b.remoteExpectedActions)-len(remoteActions),
			spew.Sdump(b.remoteExpectedActions[len(remoteActions):]))
	}
}

// checkAction verifies that expected and actual actions are equal and both have
// same attached resources
func checkAction(expected, actual core.Action, t *testing.T) {
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource) &&
		actual.GetSubresource() == expected.GetSubresource()) {
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

func getKey(foo *v1.Service, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(foo)
	if err != nil {
		t.Errorf("Unexpected error getting key for service %v: %v", foo.Name, err)
		return ""
	}
	return key
}
