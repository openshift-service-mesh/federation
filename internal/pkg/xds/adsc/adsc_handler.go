package adsc

import "google.golang.org/protobuf/types/known/anypb"

type Handler interface {
	Handle(resources []*anypb.Any) error
}
