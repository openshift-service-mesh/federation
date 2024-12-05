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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/openshift-service-mesh/federation/internal/pkg/config"
	"github.com/openshift-service-mesh/federation/internal/pkg/fds"
	"github.com/openshift-service-mesh/federation/internal/pkg/informer"
	"github.com/openshift-service-mesh/federation/internal/pkg/istio"
	"github.com/openshift-service-mesh/federation/internal/pkg/kube"
	"github.com/openshift-service-mesh/federation/internal/pkg/networking"
	"github.com/openshift-service-mesh/federation/internal/pkg/openshift"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds/adsc"
	"github.com/openshift-service-mesh/federation/internal/pkg/xds/adss"
	routev1client "github.com/openshift/client-go/route/clientset/versioned"
	istiokube "istio.io/istio/pkg/kube"
	istiolog "istio.io/istio/pkg/log"
	"istio.io/istio/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/rest"
)

var (
	// Global variables to store the parsed commandline arguments
	meshPeers, exportedServiceSet, importedServiceSet string

	loggingOptions = istiolog.DefaultOptions()
	log            = istiolog.RegisterScope("default", "default logging scope")
)

const reconnectDelay = time.Second * 5

// parseFlags parses command-line flags using the standard flag package.
func parseFlags() {
	flag.StringVar(&meshPeers, "meshPeers", "",
		"Mesh peers that include address ip/hostname to remote Peer, and the ports for dataplane and discovery")
	flag.StringVar(&exportedServiceSet, "exportedServiceSet", "",
		"ExportedServiceSet that includes selectors to match the services that will be exported")
	flag.StringVar(&importedServiceSet, "importedServiceSet", "",
		"ImportedServiceSet that includes selectors to match the services that will be imported")

	// Attach Istio logging options to the flag set
	loggingOptions.AttachFlags(func(_ *[]string, _ string, _ []string, _ string) {
		// unused and not available out-of-the box in flag package
	},
		flag.StringVar,
		flag.IntVar,
		flag.BoolVar)

	flag.Parse()
}

func main() {
	parseFlags()

	namespace := config.Namespace()

	if err := istiolog.Configure(loggingOptions); err != nil {
		log.Fatalf("failed to configure logging options: %v", err)
	}

	cfg, err := config.ParseArgs(meshPeers, exportedServiceSet, importedServiceSet)
	if err != nil {
		log.Fatalf("failed to parse configuration passed to the program arguments: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("failed to create in-cluster config: %v", err)
	}

	istioClient, err := istiokube.NewClient(istiokube.NewClientConfigForRestConfig(kubeConfig), "")
	if err != nil {
		log.Fatalf("failed to create Istio client: %v", err)
	}

	fdsPushRequests := make(chan xds.PushRequest)
	meshConfigPushRequests := make(chan xds.PushRequest)

	informerFactory := informers.NewSharedInformerFactory(istioClient.Kube(), 0)
	serviceInformer := informerFactory.Core().V1().Services().Informer()
	serviceLister := informerFactory.Core().V1().Services().Lister()
	informerFactory.Start(ctx.Done())

	serviceController, err := informer.NewResourceController(serviceInformer, corev1.Service{},
		informer.NewServiceExportEventHandler(*cfg, fdsPushRequests, meshConfigPushRequests))
	if err != nil {
		log.Fatalf("failed to create service informer: %v", err)
	}
	serviceController.RunAndWait(ctx.Done())

	importedServiceStore := fds.NewImportedServiceStore()
	istioConfigFactory := istio.NewConfigFactory(*cfg, serviceLister, importedServiceStore, namespace)

	triggerFDSPushOnNewSubscription := func() {
		fdsPushRequests <- xds.PushRequest{TypeUrl: xds.ExportedServiceTypeUrl}
	}
	federationServer := adss.NewServer(
		fdsPushRequests,
		triggerFDSPushOnNewSubscription,
		fds.NewExportedServicesGenerator(*cfg, serviceLister),
	)
	go func() {
		if err := federationServer.Run(ctx); err != nil {
			log.Fatalf("failed to start FDS server: %v", err)
		}
	}()

	var fdsClient *adsc.ADSC
	remote := cfg.MeshPeers.Remote
	if len(remote.Addresses) > 0 {
		var discoveryAddr string
		if networking.IsIP(remote.Addresses[0]) {
			discoveryAddr = fmt.Sprintf("federation-discovery-service-%s.istio-system.svc.cluster.local:15080", cfg.MeshPeers.Remote.Name)
		} else {
			discoveryAddr = fmt.Sprintf("%s:15080", remote.Addresses[0])
		}

		var err error
		fdsClient, err = adsc.New(&adsc.ADSCConfig{
			PeerName:      remote.Name,
			DiscoveryAddr: discoveryAddr,
			Authority:     fmt.Sprintf("federation-discovery-service-%s.istio-system.svc.cluster.local", cfg.MeshPeers.Remote.Name),
			InitialDiscoveryRequests: []*discovery.DiscoveryRequest{{
				TypeUrl: xds.ExportedServiceTypeUrl,
			}},
			Handlers: map[string]adsc.ResponseHandler{
				xds.ExportedServiceTypeUrl: fds.NewImportedServiceHandler(importedServiceStore, meshConfigPushRequests),
			},
			ReconnectDelay: reconnectDelay,
		})
		if err != nil {
			log.Fatalf("failed to create FDS client to remote %s: %v", remote.Name, err)
		}
	}

	reconcilers := []kube.Reconciler{
		kube.NewGatewayResourceReconciler(istioClient, istioConfigFactory),
		kube.NewServiceEntryReconciler(istioClient, istioConfigFactory),
		kube.NewWorkloadEntryReconciler(istioClient, istioConfigFactory),
		kube.NewPeerAuthResourceReconciler(istioClient, namespace),
	}
	if cfg.MeshPeers.Remote.IngressType == config.OpenShiftRouter {
		reconcilers = append(reconcilers, kube.NewDestinationRuleReconciler(istioClient, istioConfigFactory))
	}
	if cfg.MeshPeers.Local.IngressType == config.OpenShiftRouter {
		routeClient, err := routev1client.NewForConfig(kubeConfig)
		if err != nil {
			log.Fatalf("failed to create Route client: %v", err)
		}

		reconcilers = append(reconcilers, kube.NewEnvoyFilterReconciler(istioClient, istioConfigFactory))
		reconcilers = append(reconcilers, kube.NewRouteReconciler(routeClient, openshift.NewConfigFactory(*cfg, serviceLister)))
	}

	rm := kube.NewReconcilerManager(meshConfigPushRequests, reconcilers...)
	if err := rm.ReconcileAll(ctx); err != nil {
		log.Fatalf("initial Istio resource reconciliation failed: %v", err)
	}
	go rm.Start(ctx)

	if !networking.IsIP(cfg.MeshPeers.Remote.Addresses[0]) {
		go func() {
			log.Debugf("Resolving %s", remote.Addresses[0])
			lastIPs := networking.Resolve(remote.Addresses[0])
			for {
				log.Debugf("Resolving %s", remote.Addresses[0])
				ips := networking.Resolve(remote.Addresses[0])
				sort.Strings(ips)
				if !slices.Equal(lastIPs, ips) {
					log.Infof("IP addresses of %s have changed", remote.Addresses[0])
					lastIPs = ips
					meshConfigPushRequests <- xds.PushRequest{TypeUrl: xds.WorkloadEntryTypeUrl}
				}
				time.Sleep(1 * time.Second)
			}
		}()
	}

	if fdsClient != nil {
		go func() {
			if err := fdsClient.Run(ctx); err != nil {
				log.Errorf("failed to start FDS client, will reconnect in %s: %v", reconnectDelay, err)
				time.AfterFunc(reconnectDelay, func() {
					fdsClient.Restart(ctx)
				})
			}
		}()
	}

	select {}
}
