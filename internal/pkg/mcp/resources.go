package mcp

import "google.golang.org/protobuf/types/known/anypb"

type McpResources struct {
	TypeUrl   string
	Resources []*anypb.Any
}
