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

package fds

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/openshift-service-mesh/federation/internal/api/federation/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/pkg/legacy/xds"
	"github.com/openshift-service-mesh/federation/internal/pkg/legacy/xds/adsc"
)

var _ adsc.ResponseHandler = (*ImportedServiceHandler)(nil)

type ImportedServiceHandler struct {
	store        *ImportedServiceStore
	pushRequests chan<- xds.PushRequest
}

func NewImportedServiceHandler(store *ImportedServiceStore, pushRequests chan<- xds.PushRequest) *ImportedServiceHandler {
	return &ImportedServiceHandler{
		store:        store,
		pushRequests: pushRequests,
	}
}

func (h *ImportedServiceHandler) Handle(source string, resources []*anypb.Any) error {
	importedServices := make([]*v1alpha1.FederatedService, 0, len(resources))
	for _, res := range resources {
		exportedService := &v1alpha1.FederatedService{}
		if err := proto.Unmarshal(res.Value, exportedService); err != nil {
			return fmt.Errorf("unable to unmarshal exported service: %w", err)
		}
		importedServices = append(importedServices, exportedService)
	}

	h.store.Update(source, importedServices)
	// TODO: push only if current state != received imported services (this can happen on reconnection)
	h.pushRequests <- xds.PushRequest{TypeUrl: xds.ServiceEntryTypeUrl}
	h.pushRequests <- xds.PushRequest{TypeUrl: xds.WorkloadEntryTypeUrl}
	h.pushRequests <- xds.PushRequest{TypeUrl: xds.DestinationRuleTypeUrl}
	return nil
}
