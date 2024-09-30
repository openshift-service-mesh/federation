// Copyright Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kube

import (
	"context"
	"fmt"

	"github.com/jewertow/federation/internal/pkg/istio"
	"github.com/jewertow/federation/internal/pkg/xds"
	"istio.io/istio/pkg/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Reconciler = (*GatewayResourceReconciler)(nil)

type GatewayResourceReconciler struct {
	client kube.Client
	cf     *istio.ConfigFactory
}

func NewGatewayResourceReconciler(client kube.Client, cf *istio.ConfigFactory) *GatewayResourceReconciler {
	return &GatewayResourceReconciler{
		client: client,
		cf:     cf,
	}
}

func (r *GatewayResourceReconciler) GetTypeUrl() string {
	return xds.GatewayTypeUrl
}

func (r *GatewayResourceReconciler) Reconcile(ctx context.Context) error {
	gw, err := r.cf.GetIngressGateway()
	if err != nil {
		return fmt.Errorf("error generating ingress gateway: %v", err)
	}
	createdG, err := r.client.Istio().NetworkingV1alpha3().Gateways(gw.Namespace).Create(ctx, gw, metav1.CreateOptions{})
	if client.IgnoreAlreadyExists(err) != nil {
		return fmt.Errorf("failed to create ingress gateway: %v", err)
	}
	log.Infof("created ingress gateway: %v", createdG)

	return nil
}
