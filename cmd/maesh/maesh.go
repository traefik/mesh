package main

import (
	"fmt"
	stdlog "log"
	"os"

	"github.com/containous/maesh/cmd"
	"github.com/containous/maesh/cmd/prepare"
	"github.com/containous/maesh/cmd/version"
	"github.com/containous/maesh/internal/controller"
	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/maesh/internal/signals"
	"github.com/containous/traefik/v2/pkg/cli"
	log "github.com/sirupsen/logrus"
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

	pConfig := cmd.NewPrepareConfig()
	if err := cmdMaesh.AddCommand(prepare.NewCmd(pConfig, loaders)); err != nil {
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
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
	if iConfig.Debug {
		log.SetLevel(log.DebugLevel)
	}

	log.Debugln("Starting maesh prepare...")
	log.Debugf("Using masterURL: %q", iConfig.MasterURL)
	log.Debugf("Using kubeconfig: %q", iConfig.KubeConfig)

	clients, err := k8s.NewClientWrapper(iConfig.MasterURL, iConfig.KubeConfig)
	if err != nil {
		return fmt.Errorf("error building clients: %v", err)
	}

	if err = clients.CheckCluster(); err != nil {
		return fmt.Errorf("error during cluster check: %v", err)
	}

	// Create a new stop Channel
	stopCh := signals.SetupSignalHandler()
	// Create a new ctr.
	ctr := controller.NewMeshController(clients, iConfig.SMI, iConfig.DefaultMode, iConfig.Namespace, iConfig.IgnoreNamespaces)

	// run the ctr loop to process items
	if err = ctr.Run(stopCh); err != nil {
		log.Fatalf("Error running ctr: %v", err)
	}
	return nil
}
