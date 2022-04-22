//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
 * Copyright 2022 Hewlett Packard Enterprise Development LP
 * Other additional copyright holders may be indicated within.
 *
 * The entirety of this work is licensed under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 *
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
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RsyncNodeDataMovement) DeepCopyInto(out *RsyncNodeDataMovement) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RsyncNodeDataMovement.
func (in *RsyncNodeDataMovement) DeepCopy() *RsyncNodeDataMovement {
	if in == nil {
		return nil
	}
	out := new(RsyncNodeDataMovement)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RsyncNodeDataMovement) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RsyncNodeDataMovementList) DeepCopyInto(out *RsyncNodeDataMovementList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]RsyncNodeDataMovement, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RsyncNodeDataMovementList.
func (in *RsyncNodeDataMovementList) DeepCopy() *RsyncNodeDataMovementList {
	if in == nil {
		return nil
	}
	out := new(RsyncNodeDataMovementList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RsyncNodeDataMovementList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RsyncNodeDataMovementSpec) DeepCopyInto(out *RsyncNodeDataMovementSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RsyncNodeDataMovementSpec.
func (in *RsyncNodeDataMovementSpec) DeepCopy() *RsyncNodeDataMovementSpec {
	if in == nil {
		return nil
	}
	out := new(RsyncNodeDataMovementSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RsyncNodeDataMovementStatus) DeepCopyInto(out *RsyncNodeDataMovementStatus) {
	*out = *in
	in.StartTime.DeepCopyInto(&out.StartTime)
	in.EndTime.DeepCopyInto(&out.EndTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RsyncNodeDataMovementStatus.
func (in *RsyncNodeDataMovementStatus) DeepCopy() *RsyncNodeDataMovementStatus {
	if in == nil {
		return nil
	}
	out := new(RsyncNodeDataMovementStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RsyncTemplate) DeepCopyInto(out *RsyncTemplate) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RsyncTemplate.
func (in *RsyncTemplate) DeepCopy() *RsyncTemplate {
	if in == nil {
		return nil
	}
	out := new(RsyncTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RsyncTemplate) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RsyncTemplateList) DeepCopyInto(out *RsyncTemplateList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]RsyncTemplate, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RsyncTemplateList.
func (in *RsyncTemplateList) DeepCopy() *RsyncTemplateList {
	if in == nil {
		return nil
	}
	out := new(RsyncTemplateList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RsyncTemplateList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RsyncTemplateSpec) DeepCopyInto(out *RsyncTemplateSpec) {
	*out = *in
	in.Selector.DeepCopyInto(&out.Selector)
	in.Template.DeepCopyInto(&out.Template)
	if in.DisableLustreFileSystems != nil {
		in, out := &in.DisableLustreFileSystems, &out.DisableLustreFileSystems
		*out = new(bool)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RsyncTemplateSpec.
func (in *RsyncTemplateSpec) DeepCopy() *RsyncTemplateSpec {
	if in == nil {
		return nil
	}
	out := new(RsyncTemplateSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RsyncTemplateStatus) DeepCopyInto(out *RsyncTemplateStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RsyncTemplateStatus.
func (in *RsyncTemplateStatus) DeepCopy() *RsyncTemplateStatus {
	if in == nil {
		return nil
	}
	out := new(RsyncTemplateStatus)
	in.DeepCopyInto(out)
	return out
}
