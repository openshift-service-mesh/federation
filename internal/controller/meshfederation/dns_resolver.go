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

package meshfederation

import (
	"context"
	"sort"
	"time"

	"istio.io/istio/pkg/slices"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openshift-service-mesh/federation/internal/pkg/networking"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
)

func (r *Reconciler) resolveRemoteIP(ctx context.Context) {
	logger := log.FromContext(ctx)

	var prevIPs []string
	for _, remote := range r.remotes {
		prevIPs = append(prevIPs, networking.Resolve(remote.Addresses[0])...)
	}

	resolveIPs := func() {
		var currIPs []string
		for _, remote := range r.remotes {
			logger.Info("Resolving address", "remote-mesh", remote.Name)
			currIPs = append(currIPs, networking.Resolve(remote.Addresses[0])...)
		}

		sort.Strings(currIPs)
		if !slices.Equal(prevIPs, currIPs) {
			prevIPs = currIPs
			r.meshConfigPushRequests <- xds.PushRequest{TypeUrl: xds.WorkloadEntryTypeUrl}
		}
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

resolveLoop:
	for {
		select {
		case <-ctx.Done():
			break resolveLoop
		case <-ticker.C:
			resolveIPs()
		}
	}
}
