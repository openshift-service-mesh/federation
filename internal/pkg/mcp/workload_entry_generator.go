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

package mcp

import (
	"fmt"

	"google.golang.org/protobuf/types/known/anypb"
	istioconfig "istio.io/istio/pkg/config"

	"github.com/openshift-service-mesh/federation/internal/pkg/istio"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds/adss"
)

var _ adss.RequestHandler = (*WorkloadEntryGenerator)(nil)

type WorkloadEntryGenerator struct {
	istioConfigFactory *istio.ConfigFactory
}

func NewWorkloadEntryGenerator(istioConfigFactory *istio.ConfigFactory) *WorkloadEntryGenerator {
	return &WorkloadEntryGenerator{istioConfigFactory: istioConfigFactory}
}

func (s *WorkloadEntryGenerator) GetTypeUrl() string {
	return xds.WorkloadEntryTypeUrl
}

func (s *WorkloadEntryGenerator) GenerateResponse() ([]*anypb.Any, error) {
	workloadEntries, err := s.istioConfigFactory.GetWorkloadEntries()
	if err != nil {
		return nil, fmt.Errorf("failed to generate workload entries: %v", err)
	}

	var workloadEntryConfigs []*istioconfig.Config
	for _, we := range workloadEntries {
		workloadEntryConfigs = append(workloadEntryConfigs, &istioconfig.Config{
			Meta: istioconfig.Meta{
				Name:      we.Name,
				Namespace: we.Namespace,
			},
			Spec: &we.Spec,
		})
	}
	return serialize(workloadEntryConfigs...)
}
