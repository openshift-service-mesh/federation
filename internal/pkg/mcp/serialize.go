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
