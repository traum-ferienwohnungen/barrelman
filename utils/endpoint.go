package utils

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
				Port: port.TargetPort.IntVal,
				Name: port.Name,
			},
		)
	}
	return endpointPorts, nil
}

func endpointAdresses(nodes []*v1.Node) []v1.EndpointAddress {
	var endpointAddresses []v1.EndpointAddress

	for _, node := range nodes {
		if !IsNodeReady(node) {
			continue
		}

		ip, err := GetNodeInternalIP(node)
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

func EndpointSubset(service *v1.Service, nodes []*v1.Node) ([]v1.EndpointSubset, error) {
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
func NewEndpoint(service *v1.Service, nodes []*v1.Node) (*v1.Endpoints, error) {
	epSubset, err := EndpointSubset(service, nodes)
	if err != nil {
		return nil, err
	}

	result := &v1.Endpoints{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      service.GetName(),
			Namespace: service.GetNamespace(),
			Labels:    ServiceLabel,
		},
		Subsets: epSubset,
	}
	return result, nil
}
