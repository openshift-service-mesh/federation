package adsc

import "google.golang.org/protobuf/types/known/anypb"

// ResponseHandler handles response received from an XDS server.
type ResponseHandler interface {
	Handle(resources []*anypb.Any) error
}
