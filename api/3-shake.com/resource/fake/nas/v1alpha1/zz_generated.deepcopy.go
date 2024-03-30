//go:build !ignore_autogenerated

/*
 * Copyright Year The Kubernetes Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AllocatableDevice) DeepCopyInto(out *AllocatableDevice) {
	*out = *in
	if in.Fake != nil {
		in, out := &in.Fake, &out.Fake
		*out = new(AllocatableFake)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AllocatableDevice.
func (in *AllocatableDevice) DeepCopy() *AllocatableDevice {
	if in == nil {
		return nil
	}
	out := new(AllocatableDevice)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AllocatableFake) DeepCopyInto(out *AllocatableFake) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AllocatableFake.
func (in *AllocatableFake) DeepCopy() *AllocatableFake {
	if in == nil {
		return nil
	}
	out := new(AllocatableFake)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AllocatedDevices) DeepCopyInto(out *AllocatedDevices) {
	*out = *in
	if in.Fake != nil {
		in, out := &in.Fake, &out.Fake
		*out = new(AllocatedFakes)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AllocatedDevices.
func (in *AllocatedDevices) DeepCopy() *AllocatedDevices {
	if in == nil {
		return nil
	}
	out := new(AllocatedDevices)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AllocatedFake) DeepCopyInto(out *AllocatedFake) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AllocatedFake.
func (in *AllocatedFake) DeepCopy() *AllocatedFake {
	if in == nil {
		return nil
	}
	out := new(AllocatedFake)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AllocatedFakes) DeepCopyInto(out *AllocatedFakes) {
	*out = *in
	if in.Devices != nil {
		in, out := &in.Devices, &out.Devices
		*out = make([]AllocatedFake, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AllocatedFakes.
func (in *AllocatedFakes) DeepCopy() *AllocatedFakes {
	if in == nil {
		return nil
	}
	out := new(AllocatedFakes)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeAllocationState) DeepCopyInto(out *NodeAllocationState) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeAllocationState.
func (in *NodeAllocationState) DeepCopy() *NodeAllocationState {
	if in == nil {
		return nil
	}
	out := new(NodeAllocationState)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NodeAllocationState) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeAllocationStateConfig) DeepCopyInto(out *NodeAllocationStateConfig) {
	*out = *in
	if in.Owner != nil {
		in, out := &in.Owner, &out.Owner
		*out = new(v1.OwnerReference)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeAllocationStateConfig.
func (in *NodeAllocationStateConfig) DeepCopy() *NodeAllocationStateConfig {
	if in == nil {
		return nil
	}
	out := new(NodeAllocationStateConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeAllocationStateList) DeepCopyInto(out *NodeAllocationStateList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]NodeAllocationState, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeAllocationStateList.
func (in *NodeAllocationStateList) DeepCopy() *NodeAllocationStateList {
	if in == nil {
		return nil
	}
	out := new(NodeAllocationStateList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NodeAllocationStateList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeAllocationStateSpec) DeepCopyInto(out *NodeAllocationStateSpec) {
	*out = *in
	if in.AllocatableDevice != nil {
		in, out := &in.AllocatableDevice, &out.AllocatableDevice
		*out = make([]AllocatableDevice, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.AllocatedClaims != nil {
		in, out := &in.AllocatedClaims, &out.AllocatedClaims
		*out = make(map[string]AllocatedDevices, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	if in.PreparedDevices != nil {
		in, out := &in.PreparedDevices, &out.PreparedDevices
		*out = make(map[string]PreparedDevices, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeAllocationStateSpec.
func (in *NodeAllocationStateSpec) DeepCopy() *NodeAllocationStateSpec {
	if in == nil {
		return nil
	}
	out := new(NodeAllocationStateSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PreparedDevices) DeepCopyInto(out *PreparedDevices) {
	*out = *in
	if in.Fake != nil {
		in, out := &in.Fake, &out.Fake
		*out = new(PreparedFakes)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PreparedDevices.
func (in *PreparedDevices) DeepCopy() *PreparedDevices {
	if in == nil {
		return nil
	}
	out := new(PreparedDevices)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PreparedFake) DeepCopyInto(out *PreparedFake) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PreparedFake.
func (in *PreparedFake) DeepCopy() *PreparedFake {
	if in == nil {
		return nil
	}
	out := new(PreparedFake)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PreparedFakes) DeepCopyInto(out *PreparedFakes) {
	*out = *in
	if in.Devices != nil {
		in, out := &in.Devices, &out.Devices
		*out = make([]PreparedFake, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PreparedFakes.
func (in *PreparedFakes) DeepCopy() *PreparedFakes {
	if in == nil {
		return nil
	}
	out := new(PreparedFakes)
	in.DeepCopyInto(out)
	return out
}
