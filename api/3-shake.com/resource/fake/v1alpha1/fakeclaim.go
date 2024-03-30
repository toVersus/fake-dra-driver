package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FakeClaimParametersSpec struct {
	Count    int           `json:"count,omitempty"`
	Split    int           `json:"split,omitempty"`
	Selector *FakeSelector `json:"selector,omitempty"`
}

type FakeSelector struct {
	Model *string `json:"model,omitempty"`
}

// ToNamedResourcesSelector converts a FakeSelector into a selector for use with
// the NamedResources structured model
func (s FakeSelector) ToNamedResourcesSelector() string {
	if s.Model != nil {
		return fmt.Sprintf(`attributes.string["model"] == %q`, *s.Model)
	}
	return "()"
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
// +kubebuilder:resource:scope=Namespaced
//
// FakeClaimParameters holds the set of parameters provided when creating a resource claim for a Fake resource
type FakeClaimParameters struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec FakeClaimParametersSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//
// FakeClaimParametersList is a list of FakeClaimParameters resources
type FakeClaimParametersList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []FakeClaimParameters `json:"items"`
}
