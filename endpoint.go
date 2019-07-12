package main

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func endpointPorts(service *v1.Service) ([]v1.EndpointPort, error) {
	if len(service.Spec.Ports) < 1 {
		return nil, fmt.Errorf("No service ports defined for service: %s", service.GetName())
	}
	var endpointPorts []v1.EndpointPort
	for _, port := range service.Spec.Ports {
		endpointPorts = append(
			endpointPorts,
			v1.EndpointPort{
				Port: port.Port,
				Name: port.Name,
			},
		)
	}
	return endpointPorts, nil
}

func endpointAdresses(nodes []interface{}) []v1.EndpointAddress {
	var endpointAddresses []v1.EndpointAddress

	for _, obj := range nodes {
		node := obj.(*v1.Node)
		if !isNodeReady(node) {
			continue
		}

		ip, err := getNodeInternalIP(node)
		if err != nil {
			continue
		}

		endpointAddresses = append(
			endpointAddresses,
			v1.EndpointAddress{
				IP: ip,
			},
		)
	}
	return endpointAddresses
}

func endpointSubset(service *v1.Service, nodes []interface{}) ([]v1.EndpointSubset, error) {
	epPorts, err := endpointPorts(service)
	if err != nil {
		return nil, err
	}

	epAddresses := endpointAdresses(nodes)
	if len(epAddresses) < 1 {
		return nil, fmt.Errorf("No valid (ready) node IPs found")
	}
	return []v1.EndpointSubset{
		v1.EndpointSubset{
			Addresses: epAddresses,
			Ports:     epPorts,
		},
	}, nil
}

// NewEndPoints creates a new Endpoints object for the given service
func NewEndpoint(service *v1.Service, nodes []interface{}) (*v1.Endpoints, error) {
	epSubset, err := endpointSubset(service, nodes)
	if err != nil {
		return nil, err
	}

	result := &v1.Endpoints{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      service.GetName(),
			Namespace: service.GetNamespace(),
			Labels:    map[string]string{"tfw.io/upstreamwacher": "true"},
		},
		Subsets: epSubset,
	}
	return result, nil
}
