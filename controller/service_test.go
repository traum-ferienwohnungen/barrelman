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
	remoteServiceLister []*v1.Service
	localServiceLister  []*v1.Service
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

func (f *scFixture) newController(createNodePortSvc bool) (*ServiceController, kubeinformers.SharedInformerFactory, kubeinformers.SharedInformerFactory) {
	f.localClient = k8sfake.NewSimpleClientset(f.localObjects...)
	f.remoteClient = k8sfake.NewSimpleClientset(f.remoteObjects...)

	remoteServiceInformer := kubeinformers.NewSharedInformerFactory(f.remoteClient, noResyncPeriodFunc())
	localServiceInformer := kubeinformers.NewSharedInformerFactory(f.localClient, noResyncPeriodFunc())

	c := NewServiceController(
		f.localClient, f.remoteClient,
		remoteServiceInformer.Core().V1().Services(), localServiceInformer.Core().V1().Services(),
		createNodePortSvc,
	)

	c.remoteSynced = alwaysReady

	// Preload test objects into informers
	for _, s := range f.remoteServiceLister {
		err := remoteServiceInformer.Core().V1().Services().Informer().GetIndexer().Add(s)
		if err != nil {
			f.t.Errorf("Failed to add remote service: %v", err)
		}
	}
	for _, s := range f.localServiceLister {
		err := localServiceInformer.Core().V1().Services().Informer().GetIndexer().Add(s)
		if err != nil {
			f.t.Errorf("Failed to add local service: %v", err)
		}
	}

	return c, remoteServiceInformer, localServiceInformer
}

func (f *scFixture) runClusterIP(serviceName string) {
	f.runController(serviceName, false, false)
}

func (f *scFixture) runNodePort(serviceName string) {
	f.runController(serviceName, true, false)
}

func (f *scFixture) runController(serviceName string, createNodePortSvc bool, expectError bool) {
	c, rSI, lSI := f.newController(createNodePortSvc)

	stopCh := make(chan struct{})
	defer close(stopCh)
	rSI.Start(stopCh)
	lSI.Start(stopCh)

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
	f.remoteServiceLister = append(f.remoteServiceLister, remoteService)
	f.remoteObjects = append(f.remoteObjects, remoteService)

	f.expectCreateNamespaceAction(scNewNamespace())

	// Expect a service with ResourceLabel in local cluster
	localService := scNewService()
	localService.Labels = utils.ResourceLabel
	localService.Spec.Type = v1.ServiceTypeClusterIP
	localService.Spec.Ports = []v1.ServicePort{
		{
			Name:       portName,
			Port:       portNum,
			TargetPort: intstr.FromInt(portNodePort),
		},
	}
	f.expectCreateServiceAction(localService)

	f.runClusterIP(getKey(remoteService, t))
}

func TestCreatesNodePortService(t *testing.T) {
	f := newScFixture(t)

	remoteService := scNewService()
	f.remoteServiceLister = append(f.remoteServiceLister, remoteService)
	f.remoteObjects = append(f.remoteObjects, remoteService)

	f.expectCreateNamespaceAction(scNewNamespace())

	// Expect a service with ResourceLabel in local cluster
	localService := scNewService()
	localService.Labels = utils.ResourceLabel
	localService.Spec.Type = v1.ServiceTypeNodePort
	localService.Spec.Ports = []v1.ServicePort{
		{
			Name:       portName,
			Port:       portNum,
			TargetPort: intstr.FromInt(portNodePort),
			NodePort:   portNodePort,
		},
	}
	f.expectCreateServiceAction(localService)

	f.runNodePort(getKey(remoteService, t))
}

func TestDoNothing(t *testing.T) {
	f := newScFixture(t)

	remoteService := scNewService()
	f.remoteServiceLister = append(f.remoteServiceLister, remoteService)
	f.remoteObjects = append(f.remoteObjects, remoteService)

	localService := scNewService()
	localService.Labels = utils.ResourceLabel
	localService.Spec.Type = v1.ServiceTypeClusterIP
	localService.Spec.Ports = []v1.ServicePort{
		{
			Name:       portName,
			Port:       portNum,
			TargetPort: intstr.FromInt(portNodePort),
		},
	}
	f.localObjects = append(f.localObjects, localService)

	f.runClusterIP(getKey(remoteService, t))
}

func TestUpdateService(t *testing.T) {
	f := newScFixture(t)

	remoteService := scNewService()
	f.remoteServiceLister = append(f.remoteServiceLister, remoteService)
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
			Port:       portNum,
			TargetPort: intstr.FromInt(portNodePort),
		},
	}
	// The local service has a different port slice, it needs an update
	localService.Spec.Ports = []v1.ServicePort{
		{
			Name:       portName + "foooo",
			Port:       portNum,
			TargetPort: intstr.FromInt(portNodePort + 21),
		},
	}
	f.localObjects = append(f.localObjects, localService)

	f.expectUpdateServiceAction(expectService)
	f.runClusterIP(getKey(remoteService, t))
}

func TestUpdateServiceNodePort(t *testing.T) {
	f := newScFixture(t)

	remoteService := scNewService()
	f.remoteServiceLister = append(f.remoteServiceLister, remoteService)
	f.remoteObjects = append(f.remoteObjects, remoteService)

	localService := scNewService()
	localService.Labels = utils.ResourceLabel
	// Simulate an already created local service to update
	localService.Spec.Type = v1.ServiceTypeClusterIP

	// We test for type (to NodePort) and port change
	// Expect to see this local service after run
	expectService := localService.DeepCopy()
	expectService.Spec.Type = v1.ServiceTypeNodePort
	expectService.Spec.Ports = []v1.ServicePort{
		{
			Name:       portName,
			Port:       portNum,
			TargetPort: intstr.FromInt(portNodePort),
			NodePort:   portNodePort,
		},
	}

	// If the local service has a different ports, it needs an update
	localService.Spec.Ports = []v1.ServicePort{
		{
			Name:       portName + "foooo",
			Port:       portNum,
			TargetPort: intstr.FromInt(portNodePort + 21),
		},
	}
	f.localObjects = append(f.localObjects, localService)

	f.expectUpdateServiceAction(expectService)
	f.runNodePort(getKey(remoteService, t))
}

func TestDeleteService(t *testing.T) {
	f := newScFixture(t)

	remoteService := scNewService()
	remoteService.Annotations = utils.IgnoreAnnotation
	f.remoteServiceLister = append(f.remoteServiceLister, remoteService)
	f.remoteObjects = append(f.remoteObjects, remoteService)

	localService := scNewService()
	localService.Labels = utils.ResourceLabel
	f.localObjects = append(f.localObjects, localService)

	f.expectDeleteServiceAction(localService)
	f.runClusterIP(getKey(remoteService, t))
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
