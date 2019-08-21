package utils

import (
	"k8s.io/apimachinery/pkg/labels"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// LabelAnnotationKey is the key used for all barrelman specific labels and annotations
	LabelAnnotationKey = "tfw.io/barrelman"
	// LabelValueManagedResource is the label value to declare a resource as managed by barrelman (e.g. services)
	LabelValueManagedResource = "managed-resource"
	// LabelValueTrue is the label value used to mark service objects where barrelman should manage endpoints for
	LabelValueTrue = "true"
	// AnnotationValueIgnore is the annotation value used to tell barrelman to ignore a certain service
	// (ServiceController)
	AnnotationValueIgnore = "ignore"
)

var (
	// Label to be manually put on services their endpoints should be managed by barrelman.
	// (NodeEndpointController)
	ServiceLabel         = map[string]string{LabelAnnotationKey: LabelValueTrue}
	ServiceLabelSelector = metaV1.LabelSelector{
		MatchExpressions: []metaV1.LabelSelectorRequirement{{
			Key:      LabelAnnotationKey,
			Operator: metaV1.LabelSelectorOpIn,
			Values:   []string{LabelValueTrue, LabelValueManagedResource},
		}},
	}
	// ServiceSelector is converted from ServiceLabelSelector by init()
	ServiceSelector labels.Selector

	// Label for (dummy) services managed by barrelman. (ServiceController)
	ResourceLabel = map[string]string{LabelAnnotationKey: LabelValueManagedResource}

	// Annotation to be placed on service objects that should be ignored by barrelman
	// E.g. no dummy services are created for. (ServiceController)
	IgnoreAnnotation = map[string]string{LabelAnnotationKey: AnnotationValueIgnore}
)

func init() {
	// Ensure we can convert ServiceLabelSelector so label.Selector
	var err error
	ServiceSelector, err = metaV1.LabelSelectorAsSelector(&ServiceLabelSelector)
	if err != nil {
		panic(err)
	}
}
