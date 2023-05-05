package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AllocatableFake struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type AllocatableDevice struct {
	Fake *AllocatableFake `json:"fake,omitempty"`
}

func (d *AllocatableDevice) Type() string {
	if d.Fake != nil {
		return FakeDeviceType
	}

	return UnknownDeviceType
}

type AllocatedFake struct {
	UUID  string `json:"uuid,omitempty"`
	Split int    `json:"split,omitempty"`
}

type AllocatedFakes struct {
	Devices []AllocatedFake `json:"devices,omitempty"`
}

type AllocatedDevices struct {
	Fake *AllocatedFakes `json:"fake,omitempty"`
}

func (d *AllocatedDevices) Type() string {
	if d.Fake != nil {
		return FakeDeviceType
	}
	return UnknownDeviceType
}

type PreparedFake struct {
	UUID       string `json:"uuid,omitempty"`
	ParentUUID string `json:"parentUUID,omitempty"`
}

type PreparedFakes struct {
	Devices []PreparedFake `json:"devices,omitempty"`
}

type PreparedDevices struct {
	Fake *PreparedFakes `json:"fake,omitempty"`
}

func (d *PreparedDevices) Type() string {
	if d.Fake != nil {
		return FakeDeviceType
	}
	return UnknownDeviceType
}

type NodeAllocationStateSpec struct {
	AllocatableDevice []AllocatableDevice         `json:"allocatableDevice,omitempty"`
	AllocatedClaims   map[string]AllocatedDevices `json:"allocatedClaims,omitempty"`
	PreparedDevices   map[string]PreparedDevices  `json:"preparedDevices,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:resource:singular=nas
type NodeAllocationState struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeAllocationStateSpec `json:"spec,omitempty"`
	Status string                  `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NodeAllocationStateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []NodeAllocationState `json:"items"`
}
