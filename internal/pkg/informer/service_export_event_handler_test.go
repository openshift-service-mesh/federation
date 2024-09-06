package informer

import (
	"testing"
	"time"

	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/xds"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	defaultConfig = config.Federation{
		ExportedServiceSet: config.ExportedServiceSet{
			Rules: []config.Rules{{
				Type: "LabelSelector",
				LabelSelectors: []config.LabelSelectors{{
					MatchLabels: map[string]string{
						"export": "true",
					},
				}},
			}},
		},
	}
)

func TestXDSTriggers(t *testing.T) {
	testCases := []struct {
		name              string
		handlerFunc       func(handler Handler)
		isTimeoutExpected bool
	}{{
		name: "service created - does not match export rules - no XDS push expected",
		handlerFunc: func(handler Handler) {
			handler.ObjectCreated(&corev1.Service{})
		},
		isTimeoutExpected: true,
	}, {
		name: "service created - matches export rules - XDS pushes expected",
		handlerFunc: func(handler Handler) {
			handler.ObjectCreated(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":    "test",
						"export": "true",
					},
				},
			})
		},
		isTimeoutExpected: false,
	}, {
		name: "service deleted - does not match export rules - no XDS push expected",
		handlerFunc: func(handler Handler) {
			handler.ObjectDeleted(&corev1.Service{})
		},
		isTimeoutExpected: true,
	}, {
		name: "service created - matches export rules - XDS pushes expected",
		handlerFunc: func(handler Handler) {
			handler.ObjectDeleted(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":    "test",
						"export": "true",
					},
				},
			})
		},
		isTimeoutExpected: false,
	}, {
		name: "service updated - old and new do not match export rules - no XDS push expected",
		handlerFunc: func(handler Handler) {
			handler.ObjectUpdated(&corev1.Service{}, &corev1.Service{})
		},
		isTimeoutExpected: true,
	}, {
		name: "service updated - old and new match export rules - no XDS push expected",
		handlerFunc: func(handler Handler) {
			handler.ObjectUpdated(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":    "test",
						"export": "true",
					},
				},
			}, &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                    "test",
						"app.kubernetes.io/name": "test",
						"export":                 "true",
					},
				},
			})
		},
		isTimeoutExpected: true,
	}, {
		name: "service updated - new does not match export rules - XDS pushes expected",
		handlerFunc: func(handler Handler) {
			handler.ObjectUpdated(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":    "test",
						"export": "true",
					},
				},
			}, &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
			})
		},
		isTimeoutExpected: false,
	}, {
		name: "service updated - old does not match export rules - XDS pushes expected",
		handlerFunc: func(handler Handler) {
			handler.ObjectUpdated(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
			}, &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":    "test",
						"export": "true",
					},
				},
			})
		},
		isTimeoutExpected: false,
	}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fdsPushRequests := make(chan xds.PushRequest)
			mcpPushRequests := make(chan xds.PushRequest)
			handler := NewServiceExportEventHandler(defaultConfig, fdsPushRequests, mcpPushRequests)

			// ObjectCreated must be called in a goroutine, because mcpPushRequests and fdsPushRequests are unbuffered channels,
			// so they are blocked until another goroutine reads from the channels.
			go func() {
				tc.handlerFunc(handler)
			}()

			checkChannel(t, mcpPushRequests, xds.GatewayTypeUrl, tc.isTimeoutExpected)
			checkChannel(t, fdsPushRequests, xds.ExportedServiceTypeUrl, tc.isTimeoutExpected)
		})
	}
}

func checkChannel(t *testing.T, requests <-chan xds.PushRequest, expectedType string, isTimeoutExpected bool) {
	t.Helper()
	timeout := time.After(10 * time.Millisecond)
	select {
	case req := <-requests:
		if isTimeoutExpected {
			t.Errorf("expected timeout, got a push request: %s/%v", req.TypeUrl, req.Resources)
		}
		if req.TypeUrl != expectedType {
			t.Errorf("expected ExportedService but got %s", req.TypeUrl)
		}
		if req.Resources != nil {
			t.Errorf("expected nil resources but got %v", req.Resources)
		}
	case <-timeout:
		if !isTimeoutExpected {
			t.Fatal("Test timed out waiting for value to arrive on channel")
		}
	}
}
