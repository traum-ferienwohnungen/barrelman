package utils

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/api/errors"

	v1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResponsibleForService(t *testing.T) {
	tests := []struct {
		name    string
		service *v1.Service
		want    bool
	}{
		{
			"RandomAnnotation",
			&v1.Service{
				ObjectMeta: metaV1.ObjectMeta{
					Annotations: map[string]string{"tfw.io/barrelman": "fooooo"},
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			true,
		},
		{
			"Responsible",
			&v1.Service{
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			true,
		},
		{
			"Ignore",
			&v1.Service{
				ObjectMeta: metaV1.ObjectMeta{
					Annotations: IgnoreAnnotation,
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeLoadBalancer,
				},
			},
			false,
		},
		{
			"IgnoreResponsible",
			&v1.Service{
				ObjectMeta: metaV1.ObjectMeta{
					Annotations: IgnoreAnnotation,
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			false,
		},
		{
			"IgnoreResource",
			&v1.Service{
				ObjectMeta: metaV1.ObjectMeta{
					Annotations: IgnoreAnnotation,
					Labels:      ResourceLabel,
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			false,
		},
		{
			"nil",
			nil,
			false,
		},
		{
			"IgnoredNamespace",
			&v1.Service{
				ObjectMeta: metaV1.ObjectMeta{
					Namespace: "kube-system",
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResponsibleForService(tt.service); got != tt.want {
				t.Errorf("ResponsibleForService() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOwnerOfService(t *testing.T) {
	tests := []struct {
		name    string
		service *v1.Service
		want    bool
	}{
		{
			"NotOwner",
			&v1.Service{
				ObjectMeta: metaV1.ObjectMeta{
					Annotations: IgnoreAnnotation,
					Labels:      ResourceLabel,
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			true,
		},
		{
			"Owner",
			&v1.Service{
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			false,
		},
		{
			"nil",
			nil,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OwnerOfService(tt.service); got != tt.want {
				t.Errorf("OwnerOfService() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetService(t *testing.T) {
	tests := []struct {
		name        string
		getFunc     GetServiceFunc
		wantService *v1.Service
		wantExists  bool
		wantErr     bool
	}{
		{
			"NilService",
			func() (*v1.Service, error) {
				return nil, nil
			},
			nil,
			false,
			false,
		},
		{
			"IsNotFoundService",
			func() (*v1.Service, error) {
				return nil, errors.NewNotFound(schema.GroupResource{Resource: "services"}, "foobar")
			},
			nil,
			false,
			false,
		},
		{
			"IsErrService",
			func() (*v1.Service, error) {
				return nil, errors.NewBadRequest("whatever")
			},
			nil,
			false,
			true,
		},
		{
			"IsService",
			func() (*v1.Service, error) {
				return &v1.Service{}, nil
			},
			&v1.Service{},
			true,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotService, gotExists, err := GetService(tt.getFunc)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetService() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotService, tt.wantService) {
				t.Errorf("GetService() gotService = %v, want %v", gotService, tt.wantService)
			}
			if gotExists != tt.wantExists {
				t.Errorf("GetService() gotExists = %v, want %v", gotExists, tt.wantExists)
			}
		})
	}
}

func TestServicePortsEqual(t *testing.T) {
	type args struct {
		a []v1.ServicePort
		b []v1.ServicePort
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			"nil",
			args{
				nil,
				nil,
			},
			true,
		},
		{
			"OneNil",
			args{
				[]v1.ServicePort{
					{
						Name: "foo",
					},
				},
				nil,
			},
			false,
		},
		{
			"UnequalLength",
			args{
				[]v1.ServicePort{
					{
						Name: "foo",
					},
				},
				[]v1.ServicePort{
					{
						Name: "foo",
					},
					{
						Name: "foo2",
					},
				},
			},
			false,
		},
		{
			"Unequal",
			args{
				[]v1.ServicePort{
					{
						Name:     "udp-foo",
						Protocol: "udp",
						Port:     53,
					},
					{
						Name:       "tcp-foo",
						Protocol:   "tcp",
						Port:       12,
						TargetPort: intstr.IntOrString{IntVal: 31},
					},
				},
				[]v1.ServicePort{
					{
						Name:     "udp-foo",
						Protocol: "udp",
						Port:     53,
					},
					{
						Name:       "tcp-foo",
						Protocol:   "tcp",
						Port:       12,
						TargetPort: intstr.IntOrString{IntVal: 312},
					},
				},
			},
			false,
		},
		{
			"Equal",
			args{
				[]v1.ServicePort{
					{
						Name:     "udp-foo",
						Protocol: "udp",
						Port:     53,
					},
					{
						Name:       "tcp-foo",
						Protocol:   "tcp",
						Port:       12,
						TargetPort: intstr.IntOrString{IntVal: 312},
					},
				},
				[]v1.ServicePort{
					{
						Name:     "udp-foo",
						Protocol: "udp",
						Port:     53,
					},
					{
						Name:       "tcp-foo",
						Protocol:   "tcp",
						Port:       12,
						TargetPort: intstr.IntOrString{IntVal: 312},
					},
				},
			},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ServicePortsEqual(tt.args.a, tt.args.b); got != tt.want {
				t.Errorf("ServicePortsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}
