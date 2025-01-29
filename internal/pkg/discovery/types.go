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

package discovery

import "google.golang.org/protobuf/types/known/anypb"

const FederatedServiceTypeUrl = "federation.openshift-service-mesh.io/v1alpha1/FederatedService"

// PushRequest notifies ADS server that it should send DiscoveryResponse to subscribers.
type PushRequest struct {
	// TypeUrl specifies DiscoveryResponse type and must always be set.
	TypeUrl string
	// Resources contains data to be sent to subscribers.
	// If it is not set, ADS server will trigger proper request handler to generate resources of given type.
	Resources []*anypb.Any
}
