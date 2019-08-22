package utils

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"

	v1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	aService = &v1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "foo-service",
			Namespace: "foo-namespace",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "fooo",
					TargetPort: intstr.FromInt(12),
				},
			},
		},
	}
)

func Test_endpointPorts(t *testing.T) {
	tests := []struct {
		name    string
		service *v1.Service
		want    []v1.EndpointPort
		wantErr bool
	}{
		{
			"NoPort",
			&v1.Service{},
			nil,
			true,
		},
		{
			"OnePort",
			&v1.Service{

				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       "fooo",
							TargetPort: intstr.FromInt(12),
						},
					},
				},
			},
			[]v1.EndpointPort{
				{Name: "fooo", Port: 12},
			},
			false,
		},
		{
			"ManyPorts",
			&v1.Service{

				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       "fooo",
							TargetPort: intstr.FromInt(12),
						},
						{
							TargetPort: intstr.FromInt(22),
						},
					},
				},
			},
			[]v1.EndpointPort{
				{Name: "fooo", Port: 12},
				{Name: "", Port: 22},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := endpointPorts(tt.service)
			if (err != nil) != tt.wantErr {
				t.Errorf("endpointPorts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("endpointPorts() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_endpointAddresses(t *testing.T) {
	tests := []struct {
		name  string
		nodes []*v1.Node
		want  []v1.EndpointAddress
	}{
		{
			"OneReadyNode",
			[]*v1.Node{
				{
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{
							{
								Type:    v1.NodeInternalIP,
								Address: "1.2.3.4",
							},
						},
						Conditions: []v1.NodeCondition{
							{
								Type:   v1.NodeReady,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
			},
			[]v1.EndpointAddress{
				{IP: "1.2.3.4"},
			},
		},
		{
			"ManyReadyNode",
			[]*v1.Node{
				{
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{
							{
								Type:    v1.NodeInternalIP,
								Address: "1.2.3.4",
							},
						},
						Conditions: []v1.NodeCondition{
							{
								Type:   v1.NodeReady,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
				{
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{
							{
								Type:    v1.NodeInternalIP,
								Address: "5.4.3.2.1",
							},
						},
					},
				},
				{
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{},
						Conditions: []v1.NodeCondition{
							{
								Type:   v1.NodeReady,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
				{
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{
							{
								Type:    v1.NodeInternalIP,
								Address: "1.2.3.5",
							},
						},
						Conditions: []v1.NodeCondition{
							{
								Type:   v1.NodeReady,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
			},
			[]v1.EndpointAddress{
				{IP: "1.2.3.4"},
				{IP: "1.2.3.5"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := endpointAddresses(tt.nodes); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("endpointAddresses() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewEndpoint(t *testing.T) {
	type args struct {
		service *v1.Service
		nodes   []*v1.Node
	}
	tests := []struct {
		name    string
		args    args
		want    *v1.Endpoints
		wantErr bool
	}{
		{
			"NoService",
			args{
				&v1.Service{},
				[]*v1.Node{
					{
						Status: v1.NodeStatus{
							Addresses: []v1.NodeAddress{
								{
									Type:    v1.NodeInternalIP,
									Address: "1.2.3.4",
								},
							},
							Conditions: []v1.NodeCondition{
								{
									Type:   v1.NodeReady,
									Status: v1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			nil,
			true,
		},
		{
			"OnePortService",
			args{
				aService,
				[]*v1.Node{
					{
						Status: v1.NodeStatus{
							Addresses: []v1.NodeAddress{
								{
									Type:    v1.NodeInternalIP,
									Address: "1.2.3.4",
								},
							},
							Conditions: []v1.NodeCondition{
								{
									Type:   v1.NodeReady,
									Status: v1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			&v1.Endpoints{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      "foo-service",
					Namespace: "foo-namespace",
					Labels:    ServiceLabel,
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{{IP: "1.2.3.4"}},
						Ports:     []v1.EndpointPort{{Name: "fooo", Port: 12}},
					},
				},
			},
			false,
		},
		{
			"UnreadyNode",
			args{
				aService,
				[]*v1.Node{
					{
						Status: v1.NodeStatus{
							Addresses: []v1.NodeAddress{
								{
									Type:    v1.NodeInternalIP,
									Address: "1.2.3.4",
								},
							},
							Conditions: []v1.NodeCondition{
								{
									Type:   v1.NodeReady,
									Status: v1.ConditionFalse,
								},
							},
						},
					},
				},
			},
			nil,
			true,
		},
		{
			"NoNodeIP",
			args{
				aService,
				[]*v1.Node{
					{
						Status: v1.NodeStatus{
							Addresses: []v1.NodeAddress{},
							Conditions: []v1.NodeCondition{
								{
									Type:   v1.NodeReady,
									Status: v1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			nil,
			true,
		},
		{
			"NodeNetworkUnavailable",
			args{
				aService,
				[]*v1.Node{
					{
						Status: v1.NodeStatus{
							Addresses: []v1.NodeAddress{},
							Conditions: []v1.NodeCondition{
								{
									Type:   v1.NodeReady,
									Status: v1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			nil,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewEndpoint(tt.args.service, tt.args.nodes)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewEndpoint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewEndpoint() got = %v, want %v", got, tt.want)
			}
		})
	}
}
