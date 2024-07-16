package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jewertow/federation/internal/pkg/config"
	server "github.com/jewertow/federation/internal/pkg/xds"
	"log"
	"os"
	"os/signal"
	"syscall"
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
		exported config.ExportedServices
		imported config.ImportedServices
	)

	if err := unmarshalJSON(*meshPeer, &peers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mesh peers: %w", err)
	}
	if err := unmarshalJSON(*exportedServiceSet, &exported); err != nil {
		return nil, fmt.Errorf("failed to unmarshal exported services: %w", err)
	}
	if err := unmarshalJSON(*importedServiceSet, &imported); err != nil {
		return nil, fmt.Errorf("failed to unmarshal imported services: %w", err)
	}

	return &config.Federation{
		MeshPeers:          peers,
		ExportedServiceSet: exported,
		ImportedServiceSet: imported,
	}, nil
}

func main() {
	flag.Parse()
	cfg, err := parse()
	if err != nil {
		os.Exit(1)
	}
	fmt.Println("Configuration: ", cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := server.Run(ctx); err != nil {
		log.Fatal("Error starting server: ", err)
	}

	os.Exit(0)
}
