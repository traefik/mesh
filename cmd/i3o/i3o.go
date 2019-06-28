package main

import (
	"fmt"
	stdlog "log"
	"os"

	"github.com/containous/i3o/internal/controller/mesh"

	"github.com/containous/i3o/cmd"
	"github.com/containous/i3o/cmd/patch"
	"github.com/containous/i3o/cmd/version"
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

	pConfig := cmd.NewPatchConfig()
	if err := cmdI3o.AddCommand(patch.NewCmd(pConfig, loaders)); err != nil {
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

	log.Debugln("Starting i3o patch...")
	log.Debugf("Using masterURL: %q", iConfig.MasterURL)
	log.Debugf("Using kubeconfig: %q", iConfig.KubeConfig)

	clients, err := k8s.NewClientWrapper(iConfig.MasterURL, iConfig.KubeConfig)
	if err != nil {
		return fmt.Errorf("error building clients: %v", err)
	}

	// Create a new stop Channel
	stopCh := signals.SetupSignalHandler()
	// Create a new controller.
	controller := mesh.NewMeshController(clients)

	// run the controller loop to process items
	if err = controller.Run(stopCh); err != nil {
		log.Fatalf("Error running controller: %v", err)
	}
	return nil
}
