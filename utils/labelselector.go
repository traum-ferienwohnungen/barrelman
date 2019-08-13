package utils

import "k8s.io/apimachinery/pkg/labels"

// Kubernetes label set to identify barrelman controlled services
var (
	ServiceLabel         = map[string]string{"tfw.io/barrelman": "true"}
	ServiceLabelSelector = labels.Set(ServiceLabel).AsSelector()
)
