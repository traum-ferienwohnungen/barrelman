package utils

import (
	"testing"

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
					Annotations: map[string]string{"tfw.io/barrelman": "ignore"},
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
					Annotations: map[string]string{"tfw.io/barrelman": "ignore"},
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
					Annotations: map[string]string{"tfw.io/barrelman": "ignore"},
					Labels:      map[string]string{"tfw.io/barrelman-resource": "true"},
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
					Annotations: map[string]string{"tfw.io/barrelman": "ignore"},
					Labels:      map[string]string{"tfw.io/barrelman-resource": "true"},
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
