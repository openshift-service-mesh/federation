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

package xds

const (
	ExportedServiceTypeUrl    = "federation.istio-ecosystem.io/v1alpha1/ExportedService"
	DestinationRuleTypeUrl    = "networking.istio.io/v1alpha3/DestinationRule"
	GatewayTypeUrl            = "networking.istio.io/v1alpha3/Gateway"
	ServiceEntryTypeUrl       = "networking.istio.io/v1alpha3/ServiceEntry"
	WorkloadEntryTypeUrl      = "networking.istio.io/v1alpha3/WorkloadEntry"
	EnvoyFilterTypeUrl        = "networking.istio.io/v1alpha3/EnvoyFilter"
	PeerAuthenticationTypeUrl = "security.istio.io/v1/PeerAuthentication"
	RouteTypeUrl              = "route.openshift.io/v1/Route"
)
