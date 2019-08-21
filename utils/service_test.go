package utils

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/api/errors"

	v1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResponsibleForService(t *testing.T) {
	type testPair struct {
		input  *v1.Service
		output bool
	}

	var tests = []testPair{
		{
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
			&v1.Service{
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			true,
		},
		{
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
	}
	for _, test := range tests {
		output := ResponsibleForService(test.input)
		if output != test.output {
			t.Errorf("Expected %t got %t for service:\n %v", test.output, output, *test.input)
		}

	}
}

func TestOwnerOfService(t *testing.T) {
	type testPair struct {
		input  *v1.Service
		output bool
	}

	var tests = []testPair{
		{
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
			&v1.Service{
				ObjectMeta: metaV1.ObjectMeta{},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeNodePort,
				},
			},
			false,
		},
	}
	for _, test := range tests {
		output := OwnerOfService(test.input)
		if output != test.output {
			t.Errorf("Expected %t got %t for service:\n %v", test.output, output, *test.input)
		}

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
