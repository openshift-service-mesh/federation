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

var _ Reconciler = (*DestinationRuleReconciler)(nil)

type DestinationRuleReconciler struct {
	client kube.Client
	cf     *istio.ConfigFactory
}

func NewDestinationRuleReconciler(client kube.Client, cf *istio.ConfigFactory) *DestinationRuleReconciler {
	return &DestinationRuleReconciler{
		client: client,
		cf:     cf,
	}
}

func (r *DestinationRuleReconciler) GetTypeUrl() string {
	return xds.DestinationRuleTypeUrl
}

func (r *DestinationRuleReconciler) Reconcile(ctx context.Context) error {
	dr := r.cf.GetDestinationRules()
	createdDR, err := r.client.Istio().NetworkingV1alpha3().DestinationRules(dr.Namespace).Create(ctx, dr, metav1.CreateOptions{})
	if client.IgnoreAlreadyExists(err) != nil {
		return fmt.Errorf("failed to create destination rule: %v", err)
	}
	log.Infof("created destination rule: %v", createdDR)

	return nil
}
