//go:build integ
// +build integ

package e2e

import (
	"testing"

	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/check"
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

func TestMeshFederation(t *testing.T) {
	framework.NewTest(t).Run(func(ctx framework.TestContext) {
		for _, c := range ctx.Clusters() {
			if err := c.ApplyYAMLContents("istio-system", strictMTLS); err != nil {
				t.Errorf("failed to apply peer authentication in cluster %s: %v", c.Name(), err)
			}
		}

		eastCluster := ctx.Clusters()[0]
		westCluster := ctx.Clusters()[1]

		a := apps.A.Instances().ForCluster(eastCluster.Name())
		b := apps.B.Instances().ForCluster(westCluster.Name())
		a[0].CallOrFail(t, echo.CallOptions{
			To:    b,
			Port:  echo.Port{Name: "http"},
			Check: check.Status(200),
		})
	})
}
