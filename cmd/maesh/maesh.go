package main

import (
	"context"
	"fmt"
	stdlog "log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/containous/maesh/cmd"
	"github.com/containous/maesh/cmd/cleanup"
	"github.com/containous/maesh/cmd/prepare"
	"github.com/containous/maesh/cmd/version"
	"github.com/containous/maesh/pkg/api"
	"github.com/containous/maesh/pkg/controller"
	"github.com/containous/maesh/pkg/k8s"
	preparepkg "github.com/containous/maesh/pkg/prepare"
	"github.com/sirupsen/logrus"
	"github.com/traefik/paerser/cli"
)

const (
	minHTTPPort = int32(5000)
	minTCPPort  = int32(10000)
	minUDPPort  = int32(15000)
)

func main() {
	maeshConfig := cmd.NewMaeshConfiguration()
	maeshLoaders := []cli.ResourceLoader{&cmd.FileLoader{}, &cli.FlagLoader{}, &cmd.EnvLoader{}}

	cmdMaesh := &cli.Command{
		Name:          "maesh",
		Description:   `maesh`,
		Configuration: maeshConfig,
		Resources:     maeshLoaders,
		Run: func(_ []string) error {
			return maeshCommand(maeshConfig)
		},
	}

	prepareConfig := cmd.NewPrepareConfiguration()
	if err := cmdMaesh.AddCommand(prepare.NewCmd(prepareConfig, maeshLoaders)); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	cleanupConfig := cmd.NewCleanupConfiguration()
	if err := cmdMaesh.AddCommand(cleanup.NewCmd(cleanupConfig, maeshLoaders)); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	if err := cmdMaesh.AddCommand(version.NewCmd()); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	if err := cli.Execute(cmdMaesh); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	os.Exit(0)
}

func maeshCommand(config *cmd.MaeshConfiguration) error {
	ctx := cmd.ContextWithSignal(context.Background())

	log, err := cmd.NewLogger(config.LogFormat, config.LogLevel, config.Debug)
	if err != nil {
		return fmt.Errorf("could not create logger: %w", err)
	}

	log.Debug("Starting maesh prepare...")
	log.Debugf("Using masterURL: %q", config.MasterURL)
	log.Debugf("Using kubeconfig: %q", config.KubeConfig)

	clients, err := k8s.NewClient(log, config.MasterURL, config.KubeConfig)
	if err != nil {
		return fmt.Errorf("error building clients: %w", err)
	}

	prep := preparepkg.NewPrepare(log, clients)

	_, err = prep.CheckDNSProvider(ctx)
	if err != nil {
		return fmt.Errorf("no valid DNS provider found: %w", err)
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
