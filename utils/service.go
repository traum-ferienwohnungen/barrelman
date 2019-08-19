package utils

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

// ResponsibleForService checks if barrelman is responsible for this service (service is type: NodePort and not ignored)
// Will return false if service is nil
func ResponsibleForService(service *v1.Service) bool {
	if service == nil {
		return false
	}

	// Ignore all services that don't have node ports
	if service.Spec.Type != v1.ServiceTypeNodePort {
		return false
	}

	// Services with the "tfw.io/barrelman: ignore" annotation should be ignored
	for k, v := range service.Annotations {
		if k == "tfw.io/barrelman" && v == "ignore" {
			return false
		}
	}

	return true
}

// OwnerOfService checks if barrelman has created a specific service object
func OwnerOfService(service *v1.Service) bool {
	if service == nil {
		return false
	}

	// Services with the label "tfw.io/barrelman-resource: true" have been created by barrelman
	// as "dummy" service.
	for k, v := range service.Labels {
		if k == "tfw.io/barrelman-resource" && v == "true" {
			return true
		}
	}

	return false
}

// GetServiceFunc is a function returning a service pointer and an error
type GetServiceFunc func() (*v1.Service, error)

// GetService fetches a service object via getFunc
func GetService(getFunc GetServiceFunc) (service *v1.Service, exists bool, err error) {
	service, err = getFunc()
	if err != nil {
		exists = false
		if errors.IsNotFound(err) {
			err = nil
		}
	}
	exists = true
	return service, exists, err
}
