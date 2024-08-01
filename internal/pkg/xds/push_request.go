package xds

import "google.golang.org/protobuf/types/known/anypb"

// PushRequest notifies ADS server that it should send DiscoveryResponse to subscribers.
type PushRequest struct {
	// TypeUrl specifies DiscoveryResponse type and must always be set.
	TypeUrl string
	// Resources contains data to be sent to subscribers.
	// If it is not set, ADS server will trigger proper request handler to generate resources of given type.
	Resources []*anypb.Any
}
