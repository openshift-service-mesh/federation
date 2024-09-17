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

//go:build integ
// +build integ

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"istio.io/istio/pkg/test/echo/common/scheme"
	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/check"
	"istio.io/istio/pkg/test/framework/components/echo/common/ports"
	"istio.io/istio/pkg/test/framework/components/echo/match"
	"istio.io/istio/pkg/test/util/retry"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const strictMTLS = `
apiVersion: security.istio.io/v1
kind: PeerAuthentication
metadata:
  name: default
spec:
  mtls:
    mode: STRICT
`

func TestTraffic(t *testing.T) {
	framework.NewTest(t).Run(func(ctx framework.TestContext) {
		// TODO: move to TestMain
		for _, c := range ctx.Clusters() {
			if err := c.ApplyYAMLContents("istio-system", strictMTLS); err != nil {
				t.Errorf("failed to apply peer authentication in cluster %s: %v", c.Name(), err)
			}
		}

		a := match.ServiceName(echo.NamespacedName{Name: "a", Namespace: appNs}).GetMatches(eastApps)
		if len(a) == 0 {
			ctx.Fatalf("failed to find a match for a")
		}

		ctx.NewSubTest("requests to b should be routed only to local instances").Run(func(ctx framework.TestContext) {
			a[0].CallOrFail(t, echo.CallOptions{
				Address: fmt.Sprintf("b.%s.svc.cluster.local", appNs.Name()),
				Port:    ports.HTTP,
				Check:   check.And(check.OK(), check.ReachedClusters(ctx.AllClusters(), cluster.Clusters{ctx.Clusters().GetByName(eastClusterName)})),
				Count:   5,
			})
		})

		ctx.NewSubTest("requests to c should fail").Run(func(ctx framework.TestContext) {
			a[0].CallOrFail(t, echo.CallOptions{
				Address: fmt.Sprintf("c.%s.svc.cluster.local", appNs.Name()),
				Port:    ports.HTTP,
				Check:   check.Status(503),
				Timeout: 1 * time.Second,
			})
		})

		if err := exportService(ctx.Clusters().GetByName(westClusterName), "b", appNs.Name()); err != nil {
			t.Errorf("failed to export service b in cluster %s: %v", westClusterName, err)
		}
		if err := exportService(ctx.Clusters().GetByName(westClusterName), "c", appNs.Name()); err != nil {
			t.Errorf("failed to export service c in cluster %s: %v", westClusterName, err)
		}

		ctx.NewSubTest("requests to b should be routed to local and remote instances").Run(func(ctx framework.TestContext) {
			for _, host := range []string{
				fmt.Sprintf("b.%s", appNs.Name()),
				fmt.Sprintf("b.%s.svc", appNs.Name()),
				fmt.Sprintf("b.%s.svc.cluster.local", appNs.Name()),
			} {
				a[0].CallOrFail(t, echo.CallOptions{
					Address: host,
					Port:    ports.HTTP,
					Scheme:  scheme.HTTP,
					Check:   check.And(check.OK(), check.ReachedClusters(ctx.AllClusters(), ctx.AllClusters())),
					Count:   5,
				})
				a[0].CallOrFail(t, echo.CallOptions{
					Address: host,
					Port:    ports.HTTP2,
					Scheme:  scheme.HTTP,
					Check:   check.And(check.OK(), check.ReachedClusters(ctx.AllClusters(), ctx.AllClusters())),
					Count:   5,
				})
				a[0].CallOrFail(t, echo.CallOptions{
					Address: host,
					Port:    ports.HTTPS,
					Scheme:  scheme.HTTPS,
					Check:   check.And(check.OK(), check.ReachedClusters(ctx.AllClusters(), ctx.AllClusters())),
					Count:   5,
				})
				a[0].CallOrFail(t, echo.CallOptions{
					Address: host,
					Port:    ports.GRPC,
					Scheme:  scheme.GRPC,
					Check:   check.And(check.GRPCStatus(codes.OK), check.ReachedClusters(ctx.AllClusters(), ctx.AllClusters())),
					Count:   5,
				})
			}
		})

		// Services that exist only in remote clusters - created locally as ServiceEntries - cannot be accessed
		// as <service-name>.<namespace> or <service-name>.<namespace>.svc, because Istio generates clusters
		// for each hostname defined in a ServiceEntry, and SNI for TLS origination in these clusters
		// is generated from those hostnames, and at the same time, east-west gateways configure SNI routing
		// only for FQDNs, so mTLS connections to <service-name>.<namespace> or <service-name>.<namespace>.svc
		// fail, because there are no filters matching such SNIs.
		ctx.NewSubTest("requests to c should succeed").Run(func(ctx framework.TestContext) {
			a[0].CallOrFail(t, echo.CallOptions{
				Address: fmt.Sprintf("c.%s.svc.cluster.local", appNs.Name()),
				Port:    ports.HTTP,
				Scheme:  scheme.HTTP,
				Check:   check.OK(),
			})
			a[0].CallOrFail(t, echo.CallOptions{
				Address: fmt.Sprintf("c.%s.svc.cluster.local", appNs.Name()),
				Port:    ports.HTTP2,
				Scheme:  scheme.HTTP,
				Check:   check.OK(),
			})
			a[0].CallOrFail(t, echo.CallOptions{
				Address: fmt.Sprintf("c.%s.svc.cluster.local", appNs.Name()),
				Port:    ports.HTTPS,
				Scheme:  scheme.HTTPS,
				Check:   check.OK(),
			})
			a[0].CallOrFail(t, echo.CallOptions{
				Address: fmt.Sprintf("c.%s.svc.cluster.local", appNs.Name()),
				Port:    ports.GRPC,
				Scheme:  scheme.GRPC,
				Check:   check.GRPCStatus(codes.OK),
			})
		})
	})
}

func exportService(c cluster.Cluster, svcName, svcNs string) error {
	if err := retry.UntilSuccess(func() error {
		svc, err := c.Kube().CoreV1().Services(svcNs).Get(context.TODO(), svcName, v1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get service %s/%s: %v", svcNs, svcName, err)
		}
		svc.Labels["export-service"] = "true"
		_, err = c.Kube().CoreV1().Services(svcNs).Update(context.TODO(), svc, v1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update service %s/%s: %v", svcNs, svcName, err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to export service %s/%s: %v", svcNs, svcName, err)
	}
	return nil
}
