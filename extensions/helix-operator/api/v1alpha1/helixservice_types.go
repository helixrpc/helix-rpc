/*
Copyright 2026.

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
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HelixServiceSpec defines the desired state of HelixService
type HelixServiceSpec struct {
	// Replicas is the number of desired pods
	// +kubebuilder:default:=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Image is the container image for the service
	// +required
	Image string `json:"image"`

	// EnableEBPFBypass auto-injects /sys/fs/bpf mounts for zero-copy loopback optimization
	// +kubebuilder:default:=false
	// +optional
	EnableEBPFBypass bool `json:"enableEBPFBypass,omitempty"`

	// RateLimit is the maximum requests per second allowed per pod
	// +kubebuilder:default:=200
	// +optional
	RateLimit int32 `json:"rateLimit,omitempty"`
}

// HelixServiceStatus defines the observed state of HelixService.
type HelixServiceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the HelixService resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// HelixService is the Schema for the helixservices API
type HelixService struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of HelixService
	// +required
	Spec HelixServiceSpec `json:"spec"`

	// status defines the observed state of HelixService
	// +optional
	Status HelixServiceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// HelixServiceList contains a list of HelixService
type HelixServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []HelixService `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &HelixService{}, &HelixServiceList{})
		return nil
	})
}
