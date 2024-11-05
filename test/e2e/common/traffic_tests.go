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

package common

import (
	"context"
	"fmt"
	"testing"

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

func RunTrafficTests(t *testing.T, ctx framework.TestContext) {
	for _, c := range ctx.Clusters() {
		if err := c.ApplyYAMLContents("istio-system", strictMTLS); err != nil {
			t.Errorf("failed to apply peer authentication in cluster %s: %v", c.Name(), err)
		}
	}

	a := match.ServiceName(echo.NamespacedName{Name: "a", Namespace: AppNs}).GetMatches(EastApps)
	if len(a) == 0 {
		ctx.Fatalf("failed to find a match for a")
	}

	ctx.NewSubTest("requests to b should be routed only to local instances").Run(func(ctx framework.TestContext) {
		a[0].CallOrFail(t, echo.CallOptions{
			Address: fmt.Sprintf("b.%s.svc.cluster.local", AppNs.Name()),
			Port:    ports.HTTP,
			Check:   check.And(check.OK(), check.ReachedClusters(ctx.AllClusters(), cluster.Clusters{ctx.Clusters().GetByName(EastClusterName)})),
			Count:   5,
		})
	})

	ctx.NewSubTest("requests to c should fail").Run(func(ctx framework.TestContext) {
		res, err := a[0].Call(echo.CallOptions{
			Address: fmt.Sprintf("c.%s.svc.cluster.local", AppNs.Name()),
			Port:    ports.HTTP,
		})
		if err == nil || res.Responses.Len() != 0 {
			t.Fatalf("the request did not fail and got the following response: %v", res)
		}
	})

	if err := exportService(ctx.Clusters().GetByName(WestClusterName), "b", AppNs.Name()); err != nil {
		t.Errorf("failed to export service b in cluster %s: %v", WestClusterName, err)
	}
	if err := exportService(ctx.Clusters().GetByName(WestClusterName), "c", AppNs.Name()); err != nil {
		t.Errorf("failed to export service c in cluster %s: %v", WestClusterName, err)
	}

	ctx.NewSubTest("requests to b should be routed to local and remote instances").Run(func(ctx framework.TestContext) {
		reachedAllClusters := func(statusCheck echo.Checker) echo.Checker {
			return check.And(statusCheck, check.ReachedClusters(ctx.AllClusters(), ctx.AllClusters()))
		}
		for _, host := range []string{
			fmt.Sprintf("b.%s", AppNs.Name()),
			fmt.Sprintf("b.%s.svc", AppNs.Name()),
			fmt.Sprintf("b.%s.svc.cluster.local", AppNs.Name()),
		} {
			a[0].CallOrFail(t, echo.CallOptions{
				Address: host,
				Port:    ports.HTTP,
				Scheme:  scheme.HTTP,
				Check:   reachedAllClusters(check.OK()),
				Count:   5,
			})
			a[0].CallOrFail(t, echo.CallOptions{
				Address: host,
				Port:    ports.HTTP2,
				Scheme:  scheme.HTTP,
				Check:   reachedAllClusters(check.OK()),
				Count:   5,
			})
			a[0].CallOrFail(t, echo.CallOptions{
				Address: host,
				Port:    ports.HTTPS,
				Scheme:  scheme.HTTPS,
				Check:   reachedAllClusters(check.OK()),
				Count:   5,
			})
			a[0].CallOrFail(t, echo.CallOptions{
				Address: host,
				Port:    ports.GRPC,
				Scheme:  scheme.GRPC,
				Check:   reachedAllClusters(check.GRPCStatus(codes.OK)),
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
		fqdnC := fmt.Sprintf("c.%s.svc.cluster.local", AppNs.Name())
		a[0].CallOrFail(t, echo.CallOptions{
			Address: fqdnC,
			Port:    ports.HTTP,
			Scheme:  scheme.HTTP,
			Check:   check.OK(),
		})
		a[0].CallOrFail(t, echo.CallOptions{
			Address: fqdnC,
			Port:    ports.HTTP2,
			Scheme:  scheme.HTTP,
			Check:   check.OK(),
		})
		a[0].CallOrFail(t, echo.CallOptions{
			Address: fqdnC,
			Port:    ports.HTTPS,
			Scheme:  scheme.HTTPS,
			Check:   check.OK(),
		})
		a[0].CallOrFail(t, echo.CallOptions{
			Address: fqdnC,
			Port:    ports.GRPC,
			Scheme:  scheme.GRPC,
			Check:   check.GRPCStatus(codes.OK),
		})
	})
}

func exportService(c cluster.Cluster, svcName, svcNs string) error {
	if err := retry.UntilSuccess(func() error {
		svc, err := c.Kube().CoreV1().Services(svcNs).Get(context.Background(), svcName, v1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get service %s/%s: %w", svcNs, svcName, err)
		}
		svc.Labels["export-service"] = "true"
		_, err = c.Kube().CoreV1().Services(svcNs).Update(context.Background(), svc, v1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update service %s/%s: %w", svcNs, svcName, err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to export service %s/%s: %w", svcNs, svcName, err)
	}
	return nil
}
