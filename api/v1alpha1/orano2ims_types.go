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

// OranO2IMSSpec defines the desired state of OranO2IMS
type OranO2IMSSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	CloudId string `json:"cloudId"`
	//+kubebuilder:default=false
	MetadataServer bool `json:"metadataServer"`
	//BackendURL string `json:"backendURL,omitempty"`
	//BackendToken string `json:"backendToken,omitempty"`
}

type DeploymentsStatus struct {
	MetadataServerStatus string `json:"metadataServerStatus,omitempty"`
}

// OranO2IMSStatus defines the observed state of OranO2IMS
type OranO2IMSStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	DeploymentsStatus DeploymentsStatus `json:"deploymentStatus,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// OranO2IMS is the Schema for the orano2ims API
type OranO2IMS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OranO2IMSSpec   `json:"spec,omitempty"`
	Status OranO2IMSStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OranO2IMSList contains a list of OranO2IMS
type OranO2IMSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OranO2IMS `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OranO2IMS{}, &OranO2IMSList{})
}
