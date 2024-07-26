package xds

import "google.golang.org/protobuf/types/known/anypb"

type ResourceGenerator interface {
	GetTypeUrl() string
	Generate() ([]*anypb.Any, error)
}
