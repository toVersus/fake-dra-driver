package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	GroupName = "nas.fake.resource.3-shake.com"
	Version   = "v1alpha1"

	FakeDeviceType    = "fake"
	UnknownDeviceType = "unknown"

	NodeAllocationStateStatusReady    = "Ready"
	NodeAllocationStateStatusNotReady = "NotReady"
)

type NodeAllocationStateConfig struct {
	Name      string
	Namespace string
	Owner     *metav1.OwnerReference
}

func NewNodeAllocationState(config *NodeAllocationStateConfig) *NodeAllocationState {
	nascrd := &NodeAllocationState{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.Name,
			Namespace: config.Namespace,
		},
	}

	if config.Owner != nil {
		nascrd.SetOwnerReferences([]metav1.OwnerReference{*config.Owner})
	}

	return nascrd
}
