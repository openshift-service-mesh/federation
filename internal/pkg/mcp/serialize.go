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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	mcpv1alpha1 "istio.io/api/mcp/v1alpha1"
	istiocfg "istio.io/istio/pkg/config"
)

func serialize(configs ...*istiocfg.Config) ([]*anypb.Any, error) {
	var serializedResources []*anypb.Any
	for _, cfg := range configs {
		mcpResBody := &anypb.Any{}
		if err := anypb.MarshalFrom(mcpResBody, (cfg.Spec).(proto.Message), proto.MarshalOptions{}); err != nil {
			return []*anypb.Any{}, fmt.Errorf("failed to serialize object to protobuf format: %w", err)
		}
		mcpResTyped := &mcpv1alpha1.Resource{
			Metadata: &mcpv1alpha1.Metadata{
				Name: fmt.Sprintf("%s/%s", cfg.Meta.Namespace, cfg.Meta.Name),
			},
			Body: mcpResBody,
		}
		serializedResource := &anypb.Any{}
		if err := anypb.MarshalFrom(serializedResource, mcpResTyped, proto.MarshalOptions{}); err != nil {
			return []*anypb.Any{}, fmt.Errorf("failed to serialize MCP resource to protobuf format: %w", err)
		}
		serializedResources = append(serializedResources, serializedResource)
	}
	return serializedResources, nil
}
