package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
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
		&DeviceClassParameters{},
		&DeviceClassParametersList{},
		&FakeClaimParameters{},
		&FakeClaimParametersList{},
	)
	metav1.AddToGroupVersion(schema, SchemeGroupVersion)
	return nil
}
