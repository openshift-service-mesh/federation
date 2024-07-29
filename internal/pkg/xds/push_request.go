package xds

import "google.golang.org/protobuf/types/known/anypb"

type PushRequest struct {
	TypeUrl string
	Body    []*anypb.Any
}
