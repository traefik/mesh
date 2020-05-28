package main

import (
	"context"
	"fmt"
	stdlog "log"
	"os"
	"sync"
	"time"

	"github.com/containous/maesh/cmd"
	"github.com/containous/maesh/cmd/cleanup"
	"github.com/containous/maesh/cmd/prepare"
	"github.com/containous/maesh/cmd/proxy"
	"github.com/containous/maesh/cmd/version"
	"github.com/containous/maesh/pkg/api"
	"github.com/containous/maesh/pkg/controller"
	"github.com/containous/maesh/pkg/k8s"
	preparepkg "github.com/containous/maesh/pkg/prepare"
	"github.com/containous/traefik/v2/pkg/cli"
	"github.com/sirupsen/logrus"
)

func main() {
	iConfig := cmd.NewMaeshConfiguration()
	loaders := []cli.ResourceLoader{&cli.FileLoader{}, &cli.FlagLoader{}, &cli.EnvLoader{}}

	cmdMaesh := &cli.Command{
		Name:          "maesh",
		Description:   `maesh`,
		Configuration: iConfig,
		Resources:     loaders,
		Run: func(_ []string) error {
			return maeshCommand(iConfig)
		},
	}

	pConfig := cmd.NewPrepareConfiguration()
	if err := cmdMaesh.AddCommand(prepare.NewCmd(pConfig, loaders)); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	cConfig := cmd.NewCleanupConfiguration()
	if err := cmdMaesh.AddCommand(cleanup.NewCmd(cConfig, loaders)); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	if err := cmdMaesh.AddCommand(proxy.NewCmd(loaders)); err != nil {
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

func maeshCommand(iConfig *cmd.MaeshConfiguration) error {
	ctx := cmd.ContextWithSignal(context.Background())

	log, err := cmd.BuildLogger(iConfig.LogFormat, iConfig.LogLevel, iConfig.Debug)
	if err != nil {
		return fmt.Errorf("could not build logger: %w", err)
	}

	log.Debugln("Starting maesh prepare...")
	log.Debugf("Using masterURL: %q", iConfig.MasterURL)
	log.Debugf("Using kubeconfig: %q", iConfig.KubeConfig)

	clients, err := k8s.NewClient(log, iConfig.MasterURL, iConfig.KubeConfig)
	if err != nil {
		return fmt.Errorf("error building clients: %v", err)
	}

	prep := preparepkg.NewPrepare(log, clients)

	_, err = prep.CheckDNSProvider()
	if err != nil {
		return fmt.Errorf("no valid DNS provider found: %v", err)
	}

	minHTTPPort := int32(5000)
	minTCPPort := int32(10000)
	minUDPPort := int32(15000)

	if iConfig.SMI {
		log.Warnf("SMI mode is deprecated, please consider using --acl instead")
	}

	aclEnabled := iConfig.ACL || iConfig.SMI
	log.Debugf("ACL mode enabled: %t", aclEnabled)

	apiServer, err := api.NewAPI(log, iConfig.APIPort, iConfig.APIHost, clients.KubernetesClient(), iConfig.Namespace)
	if err != nil {
		return fmt.Errorf("unable to create the API server: %w", err)
	}

	ctr, err := controller.NewMeshController(clients, controller.Config{
		ACLEnabled:       aclEnabled,
		DefaultMode:      iConfig.DefaultMode,
		Namespace:        iConfig.Namespace,
		IgnoreNamespaces: iConfig.IgnoreNamespaces,
		MinHTTPPort:      minHTTPPort,
		MaxHTTPPort:      minHTTPPort + iConfig.LimitHTTPPort,
		MinTCPPort:       minTCPPort,
		MaxTCPPort:       minTCPPort + iConfig.LimitTCPPort,
		MinUDPPort:       minUDPPort,
		MaxUDPPort:       minUDPPort + iConfig.LimitUDPPort,
	}, apiServer, log)
	if err != nil {
		return fmt.Errorf("unable to create controller: %w", err)
	}

	var wg sync.WaitGroup

	apiErrCh := make(chan error)
	ctrlErrCh := make(chan error)
	ctrlStopCh := make(chan struct{})

	// Start the API server.
	wg.Add(1)

	go func() {
		defer wg.Done()

		if err := apiServer.ListenAndServe(); err != nil {
			apiErrCh <- fmt.Errorf("API has stopped unexpectedly: %w", err)
		}
	}()

	// Start the Controller.
	wg.Add(1)

	go func() {
		defer wg.Done()

		if err := ctr.Run(ctrlStopCh); err != nil {
			ctrlErrCh <- fmt.Errorf("controller has stopped unexpectedly: %w", err)
		}
	}()

	// Wait for a stop event and shutdown servers.
	select {
	case <-ctx.Done():
		ctrlStopCh <- struct{}{}

		stopAPIServer(apiServer, log)
	case err := <-apiErrCh:
		log.Error(err)
		ctrlStopCh <- struct{}{}
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
