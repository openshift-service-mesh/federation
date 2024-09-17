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

package adss

import "google.golang.org/protobuf/types/known/anypb"

// RequestHandler generates XDS response for requests from subscribers or push requests triggered by other events.
type RequestHandler interface {
	// GetTypeUrl returns supported XDS type.
	// An implementation can support only one XDS type.
	GetTypeUrl() string
	// GenerateResponse returns generated resources for requested XDS type.
	GenerateResponse() ([]*anypb.Any, error)
}
