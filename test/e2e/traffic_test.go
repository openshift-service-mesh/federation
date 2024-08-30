//go:build integ
// +build integ

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

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

		for _, port := range []echo.Port{ports.HTTP, ports.HTTPS} {
			ctx.NewSubTest(fmt.Sprintf("requests to b should be routed to local and remote instances (protocol=%s)", port.Name)).Run(func(ctx framework.TestContext) {
				for _, host := range []string{
					fmt.Sprintf("b.%s", appNs.Name()),
					fmt.Sprintf("b.%s.svc", appNs.Name()),
					fmt.Sprintf("b.%s.svc.cluster.local", appNs.Name()),
				} {
					a[0].CallOrFail(t, echo.CallOptions{
						Address: host,
						Port:    port,
						Check:   check.And(check.OK(), check.ReachedClusters(ctx.AllClusters(), ctx.AllClusters())),
						Count:   5,
					})
				}
			})

			ctx.NewSubTest(fmt.Sprintf("requests to c should succeed (protocol=%s)", port.Name)).Run(func(ctx framework.TestContext) {
				for _, host := range []string{
					fmt.Sprintf("c.%s", appNs.Name()),
					fmt.Sprintf("c.%s.svc", appNs.Name()),
					fmt.Sprintf("c.%s.svc.cluster.local", appNs.Name()),
				} {
					a[0].CallOrFail(t, echo.CallOptions{
						Address: host,
						Port:    port,
						Check:   check.OK(),
					})
				}
			})
		}
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
