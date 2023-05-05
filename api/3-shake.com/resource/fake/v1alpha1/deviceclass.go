package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeviceSelector allows one to match on a specific type of Device as part of the class
type DeviceSelector struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// DeviceClassParametersSpec is the spec for DeviceClaimParameters CRD
type DeviceClassParametersSpec struct {
	DeviceSelector []DeviceSelector `json:"deviceSelector"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
// +kubebuilder:resource:scope=Cluster
//
// DeviceClassParameters holds the set of parameters provided when creating a resource class
type DeviceClassParameters struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DeviceClassParametersSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//
// DeviceClassParametersList is a list of DeviceClassParameters resources
type DeviceClassParametersList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []DeviceClassParameters `json:"items"`
}
