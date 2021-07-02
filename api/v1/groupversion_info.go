// Package v1 contains API Schema definitions for the epic v1 API group
// +kubebuilder:object:generate=true
// +groupName=epic.acnodal.io
package v1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

const (
	// GroupName is used in our API and as a prefix for annotation and
	// label keys.
	GroupName = "epic.acnodal.io"
)

var (

	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
