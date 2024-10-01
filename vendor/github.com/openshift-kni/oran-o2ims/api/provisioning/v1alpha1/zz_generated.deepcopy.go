//go:build !ignore_autogenerated

/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterDetails) DeepCopyInto(out *ClusterDetails) {
	*out = *in
	in.ClusterProvisionStartedAt.DeepCopyInto(&out.ClusterProvisionStartedAt)
	in.NonCompliantAt.DeepCopyInto(&out.NonCompliantAt)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterDetails.
func (in *ClusterDetails) DeepCopy() *ClusterDetails {
	if in == nil {
		return nil
	}
	out := new(ClusterDetails)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterRequest) DeepCopyInto(out *ClusterRequest) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterRequest.
func (in *ClusterRequest) DeepCopy() *ClusterRequest {
	if in == nil {
		return nil
	}
	out := new(ClusterRequest)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterRequest) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterRequestList) DeepCopyInto(out *ClusterRequestList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ClusterRequest, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterRequestList.
func (in *ClusterRequestList) DeepCopy() *ClusterRequestList {
	if in == nil {
		return nil
	}
	out := new(ClusterRequestList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterRequestList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterRequestSpec) DeepCopyInto(out *ClusterRequestSpec) {
	*out = *in
	out.LocationSpec = in.LocationSpec
	in.ClusterTemplateInput.DeepCopyInto(&out.ClusterTemplateInput)
	out.Timeout = in.Timeout
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterRequestSpec.
func (in *ClusterRequestSpec) DeepCopy() *ClusterRequestSpec {
	if in == nil {
		return nil
	}
	out := new(ClusterRequestSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterRequestStatus) DeepCopyInto(out *ClusterRequestStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.ClusterDetails != nil {
		in, out := &in.ClusterDetails, &out.ClusterDetails
		*out = new(ClusterDetails)
		(*in).DeepCopyInto(*out)
	}
	if in.NodePoolRef != nil {
		in, out := &in.NodePoolRef, &out.NodePoolRef
		*out = new(NodePoolRef)
		(*in).DeepCopyInto(*out)
	}
	if in.Policies != nil {
		in, out := &in.Policies, &out.Policies
		*out = make([]PolicyDetails, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterRequestStatus.
func (in *ClusterRequestStatus) DeepCopy() *ClusterRequestStatus {
	if in == nil {
		return nil
	}
	out := new(ClusterRequestStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterTemplate) DeepCopyInto(out *ClusterTemplate) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterTemplate.
func (in *ClusterTemplate) DeepCopy() *ClusterTemplate {
	if in == nil {
		return nil
	}
	out := new(ClusterTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterTemplate) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterTemplateInput) DeepCopyInto(out *ClusterTemplateInput) {
	*out = *in
	in.ClusterInstanceInput.DeepCopyInto(&out.ClusterInstanceInput)
	in.PolicyTemplateInput.DeepCopyInto(&out.PolicyTemplateInput)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterTemplateInput.
func (in *ClusterTemplateInput) DeepCopy() *ClusterTemplateInput {
	if in == nil {
		return nil
	}
	out := new(ClusterTemplateInput)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterTemplateList) DeepCopyInto(out *ClusterTemplateList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ClusterTemplate, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterTemplateList.
func (in *ClusterTemplateList) DeepCopy() *ClusterTemplateList {
	if in == nil {
		return nil
	}
	out := new(ClusterTemplateList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterTemplateList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterTemplateSpec) DeepCopyInto(out *ClusterTemplateSpec) {
	*out = *in
	out.Templates = in.Templates
	in.InputDataSchema.DeepCopyInto(&out.InputDataSchema)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterTemplateSpec.
func (in *ClusterTemplateSpec) DeepCopy() *ClusterTemplateSpec {
	if in == nil {
		return nil
	}
	out := new(ClusterTemplateSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterTemplateStatus) DeepCopyInto(out *ClusterTemplateStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterTemplateStatus.
func (in *ClusterTemplateStatus) DeepCopy() *ClusterTemplateStatus {
	if in == nil {
		return nil
	}
	out := new(ClusterTemplateStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InputDataSchema) DeepCopyInto(out *InputDataSchema) {
	*out = *in
	in.ClusterInstanceSchema.DeepCopyInto(&out.ClusterInstanceSchema)
	in.PolicyTemplateSchema.DeepCopyInto(&out.PolicyTemplateSchema)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InputDataSchema.
func (in *InputDataSchema) DeepCopy() *InputDataSchema {
	if in == nil {
		return nil
	}
	out := new(InputDataSchema)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodePoolRef) DeepCopyInto(out *NodePoolRef) {
	*out = *in
	in.HardwareProvisioningCheckStart.DeepCopyInto(&out.HardwareProvisioningCheckStart)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodePoolRef.
func (in *NodePoolRef) DeepCopy() *NodePoolRef {
	if in == nil {
		return nil
	}
	out := new(NodePoolRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PolicyDetails) DeepCopyInto(out *PolicyDetails) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PolicyDetails.
func (in *PolicyDetails) DeepCopy() *PolicyDetails {
	if in == nil {
		return nil
	}
	out := new(PolicyDetails)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Templates) DeepCopyInto(out *Templates) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Templates.
func (in *Templates) DeepCopy() *Templates {
	if in == nil {
		return nil
	}
	out := new(Templates)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Timeout) DeepCopyInto(out *Timeout) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Timeout.
func (in *Timeout) DeepCopy() *Timeout {
	if in == nil {
		return nil
	}
	out := new(Timeout)
	in.DeepCopyInto(out)
	return out
}
