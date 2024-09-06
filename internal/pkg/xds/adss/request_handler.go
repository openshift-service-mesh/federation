package adss

import "google.golang.org/protobuf/types/known/anypb"

// RequestHandler generates XDS response for requests from subscribers or push requests triggered by other events.
type RequestHandler interface {
	// GetTypeUrl returns supported XDS type.
	// An implementation can support only one XDS type.
	GetTypeUrl() string
	// GenerateResponse returns generated resources for requested XDS type.
	GenerateResponse() ([]*anypb.Any, error)
}
