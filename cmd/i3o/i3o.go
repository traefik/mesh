package main

import (
	"fmt"
	stdlog "log"
	"os"

	"github.com/containous/i3o/cmd"
	"github.com/containous/i3o/cmd/prepare"
	"github.com/containous/i3o/cmd/version"
	"github.com/containous/i3o/internal/controller"
	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/traefik/pkg/cli"
	log "github.com/sirupsen/logrus"
	"k8s.io/sample-controller/pkg/signals"
)

func main() {
	iConfig := cmd.NewI3oConfiguration()
	loaders := []cli.ResourceLoader{&cli.FileLoader{}, &cli.FlagLoader{}, &cli.EnvLoader{}}

	cmdI3o := &cli.Command{
		Name:          "i3o",
		Description:   `i3o`,
		Configuration: iConfig,
		Resources:     loaders,
		Run: func(_ []string) error {
			return i3oCommand(iConfig)
		},
	}

	pConfig := cmd.NewPrepareConfig()
	if err := cmdI3o.AddCommand(prepare.NewCmd(pConfig, loaders)); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	if err := cmdI3o.AddCommand(version.NewCmd()); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	if err := cli.Execute(cmdI3o); err != nil {
		stdlog.Println(err)
		os.Exit(1)
	}

	os.Exit(0)
}

func i3oCommand(iConfig *cmd.I3oConfiguration) error {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
	if iConfig.Debug {
		log.SetLevel(log.DebugLevel)
	}

	log.Debugln("Starting i3o prepare...")
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
	ctr := controller.NewMeshController(clients, iConfig.SMI, iConfig.DefaultMode, iConfig.Namespace)

	// run the ctr loop to process items
	if err = ctr.Run(stopCh); err != nil {
		log.Fatalf("Error running ctr: %v", err)
	}
	return nil
}
