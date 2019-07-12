package main

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
)

// getNodeInternalIP extracts the external IP from nodes adresses
func getNodeInternalIP(node *v1.Node) (string, error) {
	for _, x := range node.Status.Addresses {
		if x.Type == v1.NodeInternalIP {
			return x.Address, nil
		}
	}
	return "", fmt.Errorf("Could not find NodeInternalIP for Node: %s", node.GetName())
}

// isNodeReady checks node conditions and tries to determine if the node is ready
func isNodeReady(node *v1.Node) bool {
	ready := false
	// iterate over all conditions (quite conservative)
	// node may only be ready if NodeReady is true to ensure we don't take "no conditions" as Ready
	for _, c := range node.Status.Conditions {
		switch c.Type {
		case v1.NodeReady:
			if c.Status == v1.ConditionTrue {
				ready = true
			}
		case v1.NodeMemoryPressure, v1.NodeDiskPressure, v1.NodePIDPressure, v1.NodeNetworkUnavailable:
			if c.Status != v1.ConditionFalse {
				ready = false
			}
		}
	}
	return ready
}
