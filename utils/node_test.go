package utils

import (
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestGetNodeInternalIP(t *testing.T) {
	tests := []struct {
		name    string
		node    *v1.Node
		want    string
		wantErr bool
	}{
		{
			"NoIP",
			&v1.Node{},
			"",
			true,
		},
		{
			"ExternalIP",
			&v1.Node{
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeExternalIP,
							Address: "1.2.3.4",
						},
					},
				},
			},
			"",
			true,
		},
		{
			"OneIP",
			&v1.Node{
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeExternalIP,
							Address: "8.8.8.8",
						},
						{
							Type:    v1.NodeInternalIP,
							Address: "1.2.3.4",
						},
					},
				},
			},
			"1.2.3.4",
			false,
		},
		{
			"ManyIPs",
			&v1.Node{
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeInternalIP,
							Address: "1.2.3.4",
						},
						{
							Type:    v1.NodeInternalIP,
							Address: "4.3.2.1",
						},
					},
				},
			},
			"1.2.3.4",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetNodeInternalIP(tt.node)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetNodeInternalIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetNodeInternalIP() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNodeReady(t *testing.T) {
	tests := []struct {
		name string
		node *v1.Node
		want bool
	}{
		{
			"NoStatus",
			&v1.Node{},
			false,
		},
		{
			"NodeReady",
			&v1.Node{
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionTrue,
						},
						{
							Type:   v1.NodeDiskPressure,
							Status: v1.ConditionTrue,
						},
						{
							Type:   v1.NodeNetworkUnavailable,
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			true,
		},
		{
			"NodeNotReady",
			&v1.Node{
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			false,
		},
		{
			"NodeNetworkUnavailable",
			&v1.Node{
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeNetworkUnavailable,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			false,
		},
		{
			"NodeNetworkOkButNoReadyCondition",
			&v1.Node{
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeNetworkUnavailable,
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNodeReady(tt.node); got != tt.want {
				t.Errorf("IsNodeReady() = %v, want %v", got, tt.want)
			}
		})
	}
}
