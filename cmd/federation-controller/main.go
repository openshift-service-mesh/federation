package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/jewertow/federation/internal/pkg/federation"
	"github.com/jewertow/federation/internal/pkg/xds/adsc"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/jewertow/federation/internal/pkg/config"
	"github.com/jewertow/federation/internal/pkg/mcp"
	"github.com/jewertow/federation/internal/pkg/xds"
	"github.com/jewertow/federation/internal/pkg/xds/adss"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// Global variable to store the parsed arguments and "flag" arguments
var (
	meshPeer = flag.String("meshPeers", "",
		"Mesh peers that include address ip/hostname to remote Peer, and the ports for dataplane and discovery")
	exportedServiceSet = flag.String("exportedServiceSet", "",
		"ExportedServiceSet that include selectors to match the services that will be exported")
	importedServiceSet = flag.String("importedServiceSet", "",
		"ImportedServiceSet that include selectors to match the services that will be imported")
)

// unmarshalJSON is a utility function to unmarshal a YAML string into a struct
// and return an error if the unmarshalling fails.
func unmarshalJSON(input string, out interface{}) error {
	if err := json.Unmarshal([]byte(input), out); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return nil
}

// Parse the command line arguments by using the flag package
// Export the parsed arguments to the AppConfig variable
func parse() (*config.Federation, error) {
	var (
		peers    config.MeshPeers
		exported config.ExportedServiceSet
		imported config.ImportedServiceSet
	)

	if err := unmarshalJSON(*meshPeer, &peers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mesh peers: %w", err)
	}
	if err := unmarshalJSON(*exportedServiceSet, &exported); err != nil {
		return nil, fmt.Errorf("failed to unmarshal exported services: %w", err)
	}
	if *importedServiceSet != "" {
		if err := unmarshalJSON(*importedServiceSet, &imported); err != nil {
			return nil, fmt.Errorf("failed to unmarshal imported services: %w", err)
		}
	}

	return &config.Federation{
		MeshPeers:          peers,
		ExportedServiceSet: exported,
		ImportedServiceSet: imported,
	}, nil
}

// Start all k8s controllers and wait for informers to be synchronized
func startControllers(
	ctx context.Context, client kubernetes.Interface, cfg *config.Federation,
	informerFactory informers.SharedInformerFactory, fdsPushRequests, mcpPushRequests chan<- xds.PushRequest,
) *mcp.Controller {
	var informersInitGroup sync.WaitGroup
	informersInitGroup.Add(1)
	serviceInformer := informerFactory.Core().V1().Services().Informer()
	serviceController, err := mcp.NewResourceController(client, serviceInformer, corev1.Service{},
		[]mcp.Handler{mcp.NewExportedServiceSetHandler(*cfg, serviceInformer, fdsPushRequests, mcpPushRequests)})
	if err != nil {
		log.Fatal("Error while creating service informer: ", err)
	}
	go serviceController.Run(ctx.Done(), &informersInitGroup)

	informersInitGroup.Wait()
	klog.Infof("All controllers have been synchronized")
	return serviceController
}

func main() {
	flag.Parse()
	cfg, err := parse()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Configuration: ", cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatal("failed to create in-cluster config: ", err)
	}
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		klog.Fatalf("failed to create Kubernetes clientset: %s", err.Error())
	}

	fdsPushRequests := make(chan xds.PushRequest)
	mcpPushRequests := make(chan xds.PushRequest)

	informerFactory := informers.NewSharedInformerFactory(clientset, 0)
	serviceController := startControllers(ctx, clientset, cfg, informerFactory, fdsPushRequests, mcpPushRequests)

	federationServer := adss.NewServer(
		&adss.ServerOpts{Port: 15020, ServerID: "federation"},
		fdsPushRequests,
		federation.NewExportedServicesGenerator(*cfg, informerFactory),
	)
	go func() {
		// TODO: graceful shutdown
		if err := federationServer.Run(ctx); err != nil {
			log.Fatal("Error starting federation server: ", err)
		}
	}()
	if len(cfg.MeshPeers.Remote.Discovery.Addresses) > 0 {
		federationClient, err := adsc.New(&adsc.ADSCConfig{
			DiscoveryAddr: fmt.Sprintf("%s:15020", cfg.MeshPeers.Remote.Discovery.Addresses[0]),
			InitialDiscoveryRequests: []*discovery.DiscoveryRequest{{
				TypeUrl: "federation.istio-ecosystem.io/v1alpha1/ExportedService",
			}},
			Handlers: map[string]adsc.ResponseHandler{
				"federation.istio-ecosystem.io/v1alpha1/ExportedService": mcp.NewImportedServiceHandler(cfg, serviceController, mcpPushRequests),
			},
		})
		go func() {
			// TODO: graceful shutdown
			if err := federationClient.Run(); err != nil {
				klog.Fatal("Error starting federation server: ", err)
			}
		}()
		if err != nil {
			klog.Fatal("Error creating adss client: ", err)
		}
	}

	mcpServer := adss.NewServer(
		&adss.ServerOpts{Port: 15010, ServerID: "mcp"},
		mcpPushRequests,
		mcp.NewGatewayResourceGenerator(*cfg, informerFactory),
	)
	if err := mcpServer.Run(ctx); err != nil {
		log.Fatal("Error running XDS server: ", err)
	}

	os.Exit(0)
}
