package controller

import (
	"barrelman/utils"
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"

	v1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

var ()

type scFixture struct {
	baseFixture

	// Objects to put in the stores
	serviceLister []*v1.Service
}

func newScFixture(t *testing.T) *scFixture {
	f := &scFixture{
		baseFixture: baseFixture{
			t:             t,
			localObjects:  []runtime.Object{},
			remoteObjects: []runtime.Object{},
			informerFilter: []filterAction{
				{"list", "services"},
				{"watch", "services"},
				{"get", "services"},
				{"get", "namespaces"},
			},
		},
	}
	return f
}

func (f *scFixture) newController() (*ServiceController, kubeinformers.SharedInformerFactory) {
	f.localClient = k8sfake.NewSimpleClientset(f.localObjects...)
	f.remoteClient = k8sfake.NewSimpleClientset(f.remoteObjects...)

	serviceInformer := kubeinformers.NewSharedInformerFactory(f.remoteClient, noResyncPeriodFunc())

	c := NewServiceController(
		f.localClient,
		f.remoteClient,
		serviceInformer.Core().V1().Services(),
	)

	c.remoteSynced = alwaysReady

	// Preload test objects into informers
	for _, s := range f.serviceLister {
		err := serviceInformer.Core().V1().Services().Informer().GetIndexer().Add(s)
		if err != nil {
			f.t.Errorf("Failed to add service: %v", err)
		}
	}

	return c, serviceInformer
}

func (f *scFixture) run(serviceName string) {
	f.runController(serviceName, false)
}

func (f *scFixture) runController(serviceName string, expectError bool) {
	c, sI := f.newController()

	stopCh := make(chan struct{})
	defer close(stopCh)
	sI.Start(stopCh)

	_, err := c.syncHandler(serviceName)
	if !expectError && err != nil {
		f.t.Errorf("error syncing foo: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing foo, got nil")
	}

	f.checkActions()
}

func (f *scFixture) expectCreateServiceAction(s *v1.Service) {
	f.localExpectedActions = append(
		f.localExpectedActions,
		core.NewCreateAction(schema.GroupVersionResource{Resource: "services"}, s.Namespace, s),
	)
}

func (f *scFixture) expectUpdateServiceAction(s *v1.Service) {
	f.localExpectedActions = append(
		f.localExpectedActions,
		core.NewUpdateAction(schema.GroupVersionResource{Resource: "services"}, s.Namespace, s),
	)
}

func (f *scFixture) expectDeleteServiceAction(s *v1.Service) {
	f.localExpectedActions = append(
		f.localExpectedActions,
		core.NewDeleteAction(schema.GroupVersionResource{Resource: "services"}, s.Namespace, s.Name),
	)
}

func (f *scFixture) expectCreateNamespaceAction(n *v1.Namespace) {
	f.localExpectedActions = append(
		f.localExpectedActions,
		core.NewCreateAction(schema.GroupVersionResource{Resource: "namespaces"}, n.Namespace, n),
	)
}

func scNewNamespace() *v1.Namespace {
	return &v1.Namespace{
		ObjectMeta: metaV1.ObjectMeta{
			Name: serviceNamespace,
		},
	}
}

func scNewService() *v1.Service {
	return &v1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      serviceName,
			Namespace: serviceNamespace,
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeNodePort,
			Ports: []v1.ServicePort{
				{
					Name:       portName,
					Port:       portNum,
					TargetPort: intstr.FromInt(portNum),
					NodePort:   portNodePort,
				},
			},
		},
	}
}

func TestCreatesService(t *testing.T) {
	f := newScFixture(t)

	remoteService := scNewService()
	f.serviceLister = append(f.serviceLister, remoteService)
	f.remoteObjects = append(f.remoteObjects, remoteService)

	f.expectCreateNamespaceAction(scNewNamespace())

	// Expect a service with ResourceLabel in local cluster
	localService := scNewService()
	localService.Labels = utils.ResourceLabel
	localService.Spec.Type = v1.ServiceTypeClusterIP
	localService.Spec.Ports = []v1.ServicePort{
		{
			Name:       portName,
			TargetPort: intstr.FromInt(portNodePort),
		},
	}
	f.expectCreateServiceAction(localService)

	f.run(getKey(remoteService, t))
}

func TestDoNothing(t *testing.T) {
	f := newScFixture(t)

	remoteService := scNewService()
	f.serviceLister = append(f.serviceLister, remoteService)
	f.remoteObjects = append(f.remoteObjects, remoteService)

	localService := scNewService()
	localService.Labels = utils.ResourceLabel
	localService.Spec.Type = v1.ServiceTypeClusterIP
	localService.Spec.Ports = []v1.ServicePort{
		{
			Name:       portName,
			TargetPort: intstr.FromInt(portNodePort),
		},
	}
	f.localObjects = append(f.localObjects, localService)

	f.run(getKey(remoteService, t))
}

func TestUpdateService(t *testing.T) {
	f := newScFixture(t)

	remoteService := scNewService()
	f.serviceLister = append(f.serviceLister, remoteService)
	f.remoteObjects = append(f.remoteObjects, remoteService)

	localService := scNewService()
	localService.Labels = utils.ResourceLabel
	// Simulate an already created local service to update
	localService.Spec.Type = v1.ServiceTypeClusterIP

	// Copy local service as expected and modify the Ports afterwards
	expectService := localService.DeepCopy()
	expectService.Spec.Ports = []v1.ServicePort{
		{
			Name:       portName,
			TargetPort: intstr.FromInt(portNodePort),
		},
	}
	// The local service has a different port slice, it needs an update
	localService.Spec.Ports = []v1.ServicePort{
		{
			Name:       portName + "foooo",
			TargetPort: intstr.FromInt(portNodePort + 21),
		},
	}
	f.localObjects = append(f.localObjects, localService)

	f.expectUpdateServiceAction(expectService)
	f.run(getKey(remoteService, t))
}

func TestDeleteService(t *testing.T) {
	f := newScFixture(t)

	remoteService := scNewService()
	remoteService.Annotations = utils.IgnoreAnnotation
	f.serviceLister = append(f.serviceLister, remoteService)
	f.remoteObjects = append(f.remoteObjects, remoteService)

	localService := scNewService()
	localService.Labels = utils.ResourceLabel
	f.localObjects = append(f.localObjects, localService)

	f.expectDeleteServiceAction(localService)
	f.run(getKey(remoteService, t))
}

func TestGetLocalAction(t *testing.T) {
	type testPair struct {
		remoteExists, localExists bool
		remote, local             *v1.Service
		output                    ActionType
	}
	var tests = []testPair{
		{
			remoteExists: false,
			remote:       &v1.Service{},
			localExists:  false,
			local:        &v1.Service{},
			output:       ActionTypeNone,
		},
		{
			remoteExists: true,
			remote: &v1.Service{
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			localExists: false,
			local:       &v1.Service{},
			output:      ActionTypeAdd,
		},
		{
			remoteExists: true,
			remote: &v1.Service{
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			localExists: true,
			local: &v1.Service{
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			output: ActionTypeNone,
		},
		{
			remoteExists: true,
			remote: &v1.Service{
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			localExists: true,
			local: &v1.Service{
				ObjectMeta: metaV1.ObjectMeta{
					Labels: utils.ResourceLabel,
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeLoadBalancer,
				},
			},
			output: ActionTypeUpdate,
		},
		{
			remoteExists: true,
			remote: &v1.Service{
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			localExists: true,
			local: &v1.Service{
				ObjectMeta: metaV1.ObjectMeta{
					Labels: utils.ResourceLabel,
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			output: ActionTypeUpdate,
		},

		{
			remoteExists: true,
			remote: &v1.Service{
				ObjectMeta: metaV1.ObjectMeta{
					Annotations: utils.IgnoreAnnotation,
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			localExists: true,
			local: &v1.Service{
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			output: ActionTypeNone,
		},
		{
			remoteExists: false,
			remote:       &v1.Service{},
			localExists:  true,
			local: &v1.Service{
				ObjectMeta: metaV1.ObjectMeta{
					Labels:      utils.ResourceLabel,
					Annotations: utils.IgnoreAnnotation,
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			output: ActionTypeNone,
		},
		{
			remoteExists: false,
			remote:       &v1.Service{},
			localExists:  true,
			local: &v1.Service{
				ObjectMeta: metaV1.ObjectMeta{
					Labels: utils.ResourceLabel,
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			output: ActionTypeDelete,
		},
	}

	for n, test := range tests {
		output := getLocalAction(test.remoteExists, test.remote, test.localExists, test.local)
		if output != test.output {
			t.Errorf("Expected '%s' got '%s' for testset #%d", test.output, output, n)
		}
	}
}
