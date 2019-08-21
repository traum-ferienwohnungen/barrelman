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
		if k == LabelAnnotationKey && v == AnnotationValueIgnore {
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

	// Services with the label "tfw.io/barrelman: managed-resource" have been created by barrelman
	// as "dummy" service.
	for k, v := range service.Labels {
		if k == LabelAnnotationKey && v == LabelValueManagedResource {
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

	if err == nil {
		// claim service does not exist if we got a nil pointer
		if service == nil {
			return nil, false, nil
		}
		return service, true, nil
	}

	// Handle not found es non error, but return exists: false
	if errors.IsNotFound(err) {
		return nil, false, nil
	}

	return nil, false, err
}
