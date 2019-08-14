package utils

import (
	v1 "k8s.io/api/core/v1"
)

// ResponsibleForService checks if barrelman is responsible for this service (service is type: NodePort and not ignored)
func ResponsibleForService(service *v1.Service) bool {
	if service.Spec.Type != v1.ServiceTypeNodePort {
		return false
	}

	for k, v := range service.Annotations {
		if k == "tfw.io/barrelman" && v == "ignore" {
			return false
		}
	}

	return true
}
