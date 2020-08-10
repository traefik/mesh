package main

import (
	"context"
	"fmt"
	stdlog "log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/traefik/mesh/cmd"
	"github.com/traefik/mesh/cmd/cleanup"
	"github.com/traefik/mesh/cmd/prepare"
	"github.com/traefik/mesh/cmd/version"
	"github.com/traefik/mesh/pkg/api"
	"github.com/traefik/mesh/pkg/controller"
	"github.com/traefik/mesh/pkg/k8s"
	"github.com/traefik/paerser/cli"
)

const (
	minHTTPPort = int32(5000)
	minTCPPort  = int32(10000)
	minUDPPort  = int32(15000)
)

func main() {
	traefikMeshConfig := cmd.NewTraefikMeshConfiguration()
	traefikMeshLoaders := []cli.ResourceLoader{&cmd.FileLoader{}, &cli.FlagLoader{}, &cmd.EnvLoader{}}

	cmdTraefikMesh := &cli.Command{
		Name:          "traefik-mesh",
		Description:   `traefik-mesh`,
		Configuration: traefikMeshConfig,
		Resources:     traefikMeshLoaders,
		Run: func(_ []string) error {
			return traefikMeshCommand(traefikMeshConfig)
		},
	}

	prepareConfig := cmd.NewPrepareConfiguration()
	if err := cmdTraefikMesh.AddCommand(prepare.NewCmd(prepareConfig, traefikMeshLoaders)); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	cleanupConfig := cmd.NewCleanupConfiguration()
	if err := cmdTraefikMesh.AddCommand(cleanup.NewCmd(cleanupConfig, traefikMeshLoaders)); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	if err := cmdTraefikMesh.AddCommand(version.NewCmd()); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	if err := cli.Execute(cmdTraefikMesh); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	os.Exit(0)
}

func traefikMeshCommand(config *cmd.TraefikMeshConfiguration) error {
	ctx := cmd.ContextWithSignal(context.Background())

	log, err := cmd.NewLogger(config.LogFormat, config.LogLevel, config.Debug)
	if err != nil {
		return fmt.Errorf("could not create logger: %w", err)
	}

	log.Debug("Starting controller...")
	log.Debugf("Using masterURL: %q", config.MasterURL)
	log.Debugf("Using kubeconfig: %q", config.KubeConfig)

	clients, err := k8s.NewClient(log, config.MasterURL, config.KubeConfig)
	if err != nil {
		return fmt.Errorf("error building clients: %w", err)
	}

	if config.SMI {
		log.Warn("SMI mode is deprecated, please consider using --acl instead")
	}

	aclEnabled := config.ACL || config.SMI
	log.Debugf("ACL mode enabled: %t", aclEnabled)

	apiServer, err := api.NewAPI(log, config.APIPort, config.APIHost, clients.KubernetesClient(), config.Namespace)
	if err != nil {
		return fmt.Errorf("unable to create the API server: %w", err)
	}

	ctr := controller.NewMeshController(clients, controller.Config{
		ACLEnabled:       aclEnabled,
		DefaultMode:      config.DefaultMode,
		Namespace:        config.Namespace,
		WatchNamespaces:  config.WatchNamespaces,
		IgnoreNamespaces: config.IgnoreNamespaces,
		MinHTTPPort:      minHTTPPort,
		MaxHTTPPort:      getMaxPort(minHTTPPort, config.LimitHTTPPort),
		MinTCPPort:       minTCPPort,
		MaxTCPPort:       getMaxPort(minTCPPort, config.LimitTCPPort),
		MinUDPPort:       minUDPPort,
		MaxUDPPort:       getMaxPort(minUDPPort, config.LimitUDPPort),
	}, apiServer, log)

	var wg sync.WaitGroup

	apiErrCh := make(chan error, 1)
	ctrlErrCh := make(chan error, 1)

	// Start the API server.
	wg.Add(1)

	go func() {
		defer wg.Done()

		if err := apiServer.ListenAndServe(); err != http.ErrServerClosed {
			apiErrCh <- fmt.Errorf("API server has stopped unexpectedly: %w", err)
		}
	}()

	// Start the Controller.
	wg.Add(1)

	go func() {
		defer wg.Done()

		if err := ctr.Run(); err != nil {
			ctrlErrCh <- fmt.Errorf("controller has stopped unexpectedly: %w", err)
		}
	}()

	// Wait for a stop event and shutdown servers.
	select {
	case <-ctx.Done():
		ctr.Shutdown()
		stopAPIServer(apiServer, log)

	case err := <-apiErrCh:
		log.Error(err)
		ctr.Shutdown()

	case err := <-ctrlErrCh:
		log.Error(err)
		stopAPIServer(apiServer, log)
	}

	wg.Wait()

	return nil
}

func stopAPIServer(apiServer *api.API, log logrus.FieldLogger) {
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := apiServer.Shutdown(stopCtx); err != nil {
		log.Errorf("Unable to stop the API server: %v", err)
	}
}

func getMaxPort(min int32, limit int32) int32 {
	return min + limit - 1
}
