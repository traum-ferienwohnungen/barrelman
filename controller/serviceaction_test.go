package controller

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type testPair struct {
	remoteExists, localExists bool
	remote, local             *v1.Service
	output                    ActionType
}

func TestGetLocalAction(t *testing.T) {
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
					Labels: map[string]string{"tfw.io/barrelman-resource": "true"},
				},
				Spec: v1.ServiceSpec{
					Type: v1.ServiceTypeLoadBalancer,
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
					Labels: map[string]string{"tfw.io/barrelman-resource": "true"},
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
					Annotations: map[string]string{"tfw.io/barrelman": "ignore"},
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
					Labels:      map[string]string{"tfw.io/barrelman-resource": "true"},
					Annotations: map[string]string{"tfw.io/barrelman": "ignore"},
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
					Labels: map[string]string{"tfw.io/barrelman-resource": "true"},
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
