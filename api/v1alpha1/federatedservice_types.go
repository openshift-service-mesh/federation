// Copyright Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FederatedServiceSpec defines the desired state of FederatedService.
type FederatedServiceSpec struct {
	// Host is a FQDN of the federated service.
	Host string `json:"host,omitempty"`

	// Ports of the federated service.
	Ports []Port `json:"ports,omitempty"`

	// Labels associated with endpoints of the federated service.
	Labels map[string]string `json:"labels,omitempty"`
}

type Port struct {
	Name     string `json:"name,omitempty"`
	Number   int32  `json:"number,omitempty"`
	Protocol string `json:"protocol,omitempty"`
}

// FederatedServiceStatus defines the observed state of FederatedService.
type FederatedServiceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced

// FederatedService is the Schema for the federatedservices API.
type FederatedService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FederatedServiceSpec   `json:"spec,omitempty"`
	Status FederatedServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FederatedServiceList contains a list of FederatedService.
type FederatedServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FederatedService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FederatedService{}, &FederatedServiceList{})
}
