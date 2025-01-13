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
	routev1client "github.com/openshift/client-go/route/clientset/versioned"
	istiokube "istio.io/istio/pkg/kube"
	istiolog "istio.io/istio/pkg/log"
	"istio.io/istio/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	v1 "k8s.io/client-go/listers/core/v1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	// +kubebuilder:scaffold:imports
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/openshift-service-mesh/federation/api/v1alpha1"
	"github.com/openshift-service-mesh/federation/internal/controller"
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
)

var (
	// Global variables to store the parsed commandline arguments
	meshPeers, exportedServiceSet, importedServiceSet,
	metricsAddr, probeAddr string
	enableLeaderElection bool

	loggingOptions = istiolog.DefaultOptions()
	log            = istiolog.RegisterScope("default", "default logging scope")

	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

const reconnectDelay = time.Second * 5

// parseFlags parses command-line flags using the standard flag package.
func parseFlags() {
	flag.StringVar(&meshPeers, "meshPeers", "",
		"Mesh peers that include address ip/hostname to remote Peer, and the ports for dataplane and discovery")
	flag.StringVar(&exportedServiceSet, "exportedServiceSet", "",
		"ExportedServiceSet that includes selectors to match the services that will be exported")
	flag.StringVar(&importedServiceSet, "importedServiceSet", "",
		"ImportedServiceSet that includes selectors to match the services that will be imported")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

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
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	parseFlags()

	if err := istiolog.Configure(loggingOptions); err != nil {
		log.Fatalf("failed to configure logging options: %v", err)
	}

	cfg, err := config.ParseArgs(meshPeers, exportedServiceSet, importedServiceSet)
	if err != nil {
		log.Fatalf("failed to parse configuration passed to the program arguments: %v", err)
	}

	// TODO: investigate how we can unify logger
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "80807133.federation.openshift-service-mesh.io",
	})
	if err != nil {
		log.Errorf("unable to start manager: %s", err)
		os.Exit(1)
	}

	if err = (&controller.MeshFederationReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		log.Errorf("unable to create controller for MeshFederation custom resource: %s", err)
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Errorf("unable to set up health check: %s", err)
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Errorf("unable to set up ready check: %s", err)
		os.Exit(1)
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

	startFederationServer(ctx, cfg, serviceLister, fdsPushRequests)

	if cfg.MeshPeers.Local.IngressType == config.OpenShiftRouter {
		go resolveRemoteIP(ctx, cfg.MeshPeers.Remotes, meshConfigPushRequests)
	}

	importedServiceStore := fds.NewImportedServiceStore()
	for _, remote := range cfg.MeshPeers.Remotes {
		startFDSClient(ctx, remote, meshConfigPushRequests, importedServiceStore)
	}

	startReconciler(ctx, cfg, serviceLister, meshConfigPushRequests, importedServiceStore)

	log.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		log.Errorf("failed to start manager: %s", err)
		cancel()
	}

	<-ctx.Done()
}

func startReconciler(ctx context.Context, cfg *config.Federation, serviceLister v1.ServiceLister, meshConfigPushRequests chan xds.PushRequest, importedServiceStore *fds.ImportedServiceStore) {

	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("failed to create in-cluster config: %v", err)
	}

	istioClient, err := istiokube.NewClient(istiokube.NewClientConfigForRestConfig(kubeConfig), "")
	if err != nil {
		log.Fatalf("failed to create Istio client: %v", err)
	}

	namespace := cfg.Namespace()

	istioConfigFactory := istio.NewConfigFactory(*cfg, serviceLister, importedServiceStore, namespace)
	reconcilers := []kube.Reconciler{
		kube.NewGatewayResourceReconciler(istioClient, istioConfigFactory),
		kube.NewServiceEntryReconciler(istioClient, istioConfigFactory),
		kube.NewWorkloadEntryReconciler(istioClient, istioConfigFactory),
		kube.NewPeerAuthResourceReconciler(istioClient, namespace),
	}

	if cfg.MeshPeers.AnyRemotePeerWithOpenshiftRouterIngress() {
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
}

func startFederationServer(ctx context.Context, cfg *config.Federation, serviceLister v1.ServiceLister, fdsPushRequests chan xds.PushRequest) {
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
}

func resolveRemoteIP(ctx context.Context, remotes []config.Remote, meshConfigPushRequests chan xds.PushRequest) {
	var prevIPs []string
	for _, remote := range remotes {
		prevIPs = append(prevIPs, networking.Resolve(remote.Addresses[0])...)
	}

	resolveIPs := func() {
		var currIPs []string
		for _, remote := range remotes {
			log.Debugf("Resolving %s", remote.Name)
			currIPs = append(currIPs, networking.Resolve(remote.Addresses[0])...)
		}

		sort.Strings(currIPs)
		if !slices.Equal(prevIPs, currIPs) {
			log.Infof("IP addresses have changed")
			prevIPs = currIPs
			meshConfigPushRequests <- xds.PushRequest{TypeUrl: xds.WorkloadEntryTypeUrl}
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

func startFDSClient(ctx context.Context, remote config.Remote, meshConfigPushRequests chan xds.PushRequest, importedServiceStore *fds.ImportedServiceStore) {
	var discoveryAddr string
	if networking.IsIP(remote.Addresses[0]) {
		discoveryAddr = fmt.Sprintf("%s:%d", remote.ServiceFQDN(), remote.ServicePort())
	} else {
		discoveryAddr = fmt.Sprintf("%s:%d", remote.Addresses[0], remote.ServicePort())
	}

	fdsClient, errNew := adsc.New(&adsc.ADSCConfig{
		PeerName:      remote.Name,
		DiscoveryAddr: discoveryAddr,
		Authority:     remote.ServiceFQDN(),
		InitialDiscoveryRequests: []*discovery.DiscoveryRequest{{
			TypeUrl: xds.ExportedServiceTypeUrl,
		}},
		Handlers: map[string]adsc.ResponseHandler{
			xds.ExportedServiceTypeUrl: fds.NewImportedServiceHandler(importedServiceStore, meshConfigPushRequests),
		},
		ReconnectDelay: reconnectDelay,
	})
	if errNew != nil {
		log.Fatalf("failed to create FDS client: %v", errNew)
	}

	go func() {
		if errRun := fdsClient.Run(ctx); errRun != nil {
			log.Errorf("failed to start FDS client, will reconnect in %s: %v", reconnectDelay, errRun)
			time.AfterFunc(reconnectDelay, func() {
				if ctxErr := ctx.Err(); ctxErr != nil {
					log.Infof("Parent ctx is done: %v", ctxErr)
					return
				}

				fdsClient.Restart(ctx)
			})
		}
	}()
}
