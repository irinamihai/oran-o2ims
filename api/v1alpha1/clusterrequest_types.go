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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ClusterRequestSpec defines the desired state of ClusterRequest
type ClusterRequestSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Reference to an existing clusterTemplate CR.
	ClusterTemplateRef string `json:"clusterTemplateRef"`
	// JSON Schema input used for the clusterTemplateRef.
	ClusterTemplateInput string `json:"clusterTemplateInput"`
}

type ClusterTemplateInputValidation struct {
	// Says if the ClusterTemplateInput is valid or not.
	InputIsValid bool `json:"inputIsValid"`
	// Holds the error in case the ClusterTemplateInput is invalid.
	InputError string `json:"inputError,omitempty"`
	// Says if the provided ClusterTemplateInput matches the referenced ClusterTemplate.
	InputMatchesTemplate bool `json:"inputMatchesTemplate"`
	// Holds the error if the provided ClusterTemplateInput matches the referenced ClusterTemplate.
	InputMatchesTemplateError string `json:"inputMatchesTemplateError,omitempty"`
}

// ClusterRequestStatus defines the observed state of ClusterRequest
type ClusterRequestStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Contains JSON schema and cluster template validation details.
	ClusterTemplateInputValidation ClusterTemplateInputValidation `json:"clusterTemplateInputValidation,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ClusterRequest is the Schema for the clusterrequests API
type ClusterRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterRequestSpec   `json:"spec,omitempty"`
	Status ClusterRequestStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterRequestList contains a list of ClusterRequest
type ClusterRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterRequest{}, &ClusterRequestList{})
}
