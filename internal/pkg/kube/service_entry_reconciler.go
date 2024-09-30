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

var _ Reconciler = (*ServiceEntryReconciler)(nil)

type ServiceEntryReconciler struct {
	client kube.Client
	cf     *istio.ConfigFactory
}

func NewServiceEntryReconciler(client kube.Client, cf *istio.ConfigFactory) *ServiceEntryReconciler {
	return &ServiceEntryReconciler{
		client: client,
		cf:     cf,
	}
}

func (r *ServiceEntryReconciler) GetTypeUrl() string {
	return xds.ServiceEntryTypeUrl
}

func (r *ServiceEntryReconciler) Reconcile(ctx context.Context) error {
	serviceEntries, err := r.cf.GetServiceEntries()
	if err != nil {
		return fmt.Errorf("error generating service entries: %v", err)
	}

	for _, se := range serviceEntries {
		createdSE, err := r.client.Istio().NetworkingV1alpha3().ServiceEntries(se.Namespace).Create(ctx, se, metav1.CreateOptions{})
		if client.IgnoreAlreadyExists(err) != nil {
			return fmt.Errorf("failed to create service entry: %v", err)
		}
		log.Infof("created service entry: %v", createdSE)
	}
	return nil
}
