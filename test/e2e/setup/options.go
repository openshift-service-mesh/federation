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

package setup

import (
	"fmt"
	"time"

	"istio.io/api/annotation"
	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/common/ports"
	"istio.io/istio/pkg/test/util/retry"
)

// DeployOption can be used to enhance configuration of Echo test app
// for the test suite setup. Passing an implementation allows to
// dynamically define config enrichments without a need of introducing
// additional logic to the core DeployEcho function.
type DeployOption interface {
	ApplyToEcho(appConfig *echo.Config)
}

// WithSpire configures spire integration for the deployed app.
type WithSpire struct{}

func (w WithSpire) ApplyToEcho(appConfig *echo.Config) {
	appConfig.Subsets = []echo.SubsetConfig{{
		Annotations: map[string]string{
			annotation.InjectTemplates.Name: "sidecar,spire",
		},
	}}
}

// WithAllPorts configures all relevant ports for the application under test.
type WithAllPorts struct{}

func (a WithAllPorts) ApplyToEcho(appConfig *echo.Config) {
	appConfig.Ports = echo.Ports{
		ports.HTTP,
		ports.GRPC,
		ports.HTTP2,
		ports.HTTPS,
	}
}

// CtrlOption can alter how federation controller is configured.
type CtrlOption interface {
	ApplyToCmd(remoteCluster *Cluster, args []string) ([]string, error)
}

type RemoteAddressDNSName struct{}

func (r RemoteAddressDNSName) ApplyToCmd(remoteCluster *Cluster, args []string) ([]string, error) {
	return append(args, "--set", fmt.Sprintf("federation.meshPeers.remote.addresses[0]=ingress.%s", remoteCluster.ContextName)), nil
}

func (w WithSpire) ApplyToCmd(_ *Cluster, args []string) ([]string, error) {
	return append(args, "--set", "istio.spire.enabled=true"), nil
}

type RemoteAddressIngressIP struct{}

func (r RemoteAddressIngressIP) ApplyToCmd(remoteCluster *Cluster, args []string) ([]string, error) {
	ips, err := getIngressIPs([]cluster.Cluster{remoteCluster.Cluster})
	if err != nil {
		return nil, err
	}

	return append(args, "--set", fmt.Sprintf("federation.meshPeers.remote.addresses[0]=%s", ips[remoteCluster.Name()])), nil
}

func getIngressIPs(clusters cluster.Clusters) (map[string]string, error) {
	ingressIPs := make(map[string]string, len(clusters))

	for _, c := range clusters {
		appCluster := Resolve(c)
		if err := retry.UntilSuccess(func() error {
			gatewayIP, err := GetLoadBalancerIP(appCluster, "federation-ingress-gateway", "istio-system")
			if err != nil {
				return fmt.Errorf("could not get IPs from remote federation-controller: %w", err)
			}

			ingressIPs[appCluster.Name()] = gatewayIP

			return nil
		}, retry.Timeout(2*time.Minute), retry.Delay(200*time.Millisecond)); err != nil {
			return ingressIPs, fmt.Errorf("could not update federation-controller (cluster=%s): %w", appCluster.ContextName, err)
		}
	}

	return ingressIPs, nil
}
