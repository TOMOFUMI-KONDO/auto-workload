/*
Copyright 2022.

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

package v1beta1

import (
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AutoWorkloadSpec defines the desired state of AutoWorkload
type AutoWorkloadSpec struct {
	// Template of Workload Resource
	Template *appsv1.Deployment `json:"template,omitempty"`

	// StartAt is time to start Workload Resource
	StartAt string `json:"startAt,omitempty"`

	// StopAt is time to stop Workload Resource
	StopAt string `json:"stopAt,omitempty"`
}

// AutoWorkloadStatus defines the observed state of AutoWorkload
type AutoWorkloadStatus struct {
	// NextStartAt is next time to start Workload Resource
	NextStartAt *metav1.Time `json:"NextStartAt,omitempty"`

	// NextStopAt is next time to stop Workload Resource
	NextStopAt *metav1.Time `json:"NextStopAt,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:shortName="awl"
//+kubebuilder:printcolumn:name="START_AT",type="string",JSONPath=".spec.startAt"
//+kubebuilder:printcolumn:name="STOP_AT",type="string",JSONPath=".spec.stopAt"

// AutoWorkload is the Schema for the autoworkloads API
type AutoWorkload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutoWorkloadSpec   `json:"spec,omitempty"`
	Status AutoWorkloadStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AutoWorkloadList contains a list of AutoWorkload
type AutoWorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AutoWorkload `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AutoWorkload{}, &AutoWorkloadList{})
}
