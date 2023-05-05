package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	SchemaBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemaBuilder.AddToScheme
)

var SchemeGroupVersion = schema.GroupVersion{
	Group:   GroupName,
	Version: Version,
}

func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

func addKnownTypes(schema *runtime.Scheme) error {
	schema.AddKnownTypes(SchemeGroupVersion,
		&NodeAllocationState{},
		&NodeAllocationStateList{},
	)
	metav1.AddToGroupVersion(schema, SchemeGroupVersion)
	return nil
}
