package mcp

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	mcpv1alpha1 "istio.io/api/mcp/v1alpha1"
)

type mcpResource struct {
	name      string
	namespace string
	object    proto.Message
}

func serialize(mcpResources ...mcpResource) ([]*anypb.Any, error) {
	var serializedResources []*anypb.Any
	for _, mcpRes := range mcpResources {
		mcpResBody := &anypb.Any{}
		if err := anypb.MarshalFrom(mcpResBody, mcpRes.object, proto.MarshalOptions{}); err != nil {
			return []*anypb.Any{}, fmt.Errorf("failed to serialize object to protobuf format: %w", err)
		}
		mcpResTyped := &mcpv1alpha1.Resource{
			Metadata: &mcpv1alpha1.Metadata{
				Name: fmt.Sprintf("%s/%s", mcpRes.namespace, mcpRes.name),
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
