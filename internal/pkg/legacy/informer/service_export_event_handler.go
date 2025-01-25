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

package informer

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift-service-mesh/federation/internal/pkg/common"
	"github.com/openshift-service-mesh/federation/internal/pkg/config"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
)

var _ Handler = (*ServiceExportEventHandler)(nil)

// ServiceExportEventHandler processes Service events and triggers proper FDS/MCP pushes if an event matches export rules.
type ServiceExportEventHandler struct {
	cfg             config.Federation
	fdsPushRequests chan<- xds.PushRequest
	mcpPushRequests chan<- xds.PushRequest
}

func NewServiceExportEventHandler(
	cfg config.Federation,
	fdsPushRequests,
	mcpPushRequests chan<- xds.PushRequest,
) *ServiceExportEventHandler {
	return &ServiceExportEventHandler{
		cfg:             cfg,
		fdsPushRequests: fdsPushRequests,
		mcpPushRequests: mcpPushRequests,
	}
}

func (w *ServiceExportEventHandler) Init() error {
	return nil
}

func (w *ServiceExportEventHandler) ObjectCreated(obj runtime.Object) {
	service := obj.(*corev1.Service)
	log.Debugf("Created service %s, namespace %s", service.Name, service.Namespace)
	w.triggerXDSPushIfMatchRules(service)
}

func (w *ServiceExportEventHandler) ObjectDeleted(obj runtime.Object) {
	service := obj.(*corev1.Service)
	log.Debugf("Deleted service %s, namespace %s", service.Name, service.Namespace)
	w.triggerXDSPushIfMatchRules(service)
}

func (w *ServiceExportEventHandler) ObjectUpdated(oldObj, newObj runtime.Object) {
	oldService := oldObj.(*corev1.Service)
	newService := newObj.(*corev1.Service)
	log.Debugf("Updated service %s, namespace %s", oldService.Name, oldService.Namespace)
	w.triggerXDSPushIfMatchRules(oldService, newService)
}

func (w *ServiceExportEventHandler) triggerXDSPushIfMatchRules(services ...*corev1.Service) {
	exportLabels := w.cfg.ExportedServiceSet.GetLabelSelectors()
	if len(services) == 2 {
		if common.MatchExportRules(services[0], exportLabels) != common.MatchExportRules(services[1], exportLabels) {
			w.triggerXDSPush()
		}
	} else {
		if common.MatchExportRules(services[0], exportLabels) {
			w.triggerXDSPush()
		}
	}
}

func (w *ServiceExportEventHandler) triggerXDSPush() {
	w.mcpPushRequests <- xds.PushRequest{TypeUrl: xds.GatewayTypeUrl}
	w.mcpPushRequests <- xds.PushRequest{TypeUrl: xds.EnvoyFilterTypeUrl}
	w.mcpPushRequests <- xds.PushRequest{TypeUrl: xds.RouteTypeUrl}
	w.fdsPushRequests <- xds.PushRequest{TypeUrl: xds.ExportedServiceTypeUrl}
}
