package main

import (
	"fmt"
	stdlog "log"
	"os"

	"github.com/containous/maesh/cmd"
	"github.com/containous/maesh/cmd/prepare"
	"github.com/containous/maesh/cmd/proxy"
	"github.com/containous/maesh/cmd/version"
	"github.com/containous/maesh/pkg/controller"
	"github.com/containous/maesh/pkg/k8s"
	preparepkg "github.com/containous/maesh/pkg/prepare"
	"github.com/containous/maesh/pkg/signals"
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

	pConfig := cmd.NewPrepareConfig()
	if err := cmdMaesh.AddCommand(prepare.NewCmd(pConfig, loaders)); err != nil {
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
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.InfoLevel)

	if iConfig.Debug {
		log.SetLevel(logrus.DebugLevel)
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

	// Create a new stop Channel
	stopCh := signals.SetupSignalHandler()
	// Create a new ctr.
	ctr, err := controller.NewMeshController(clients, controller.Config{
		ACLEnabled:       aclEnabled,
		DefaultMode:      iConfig.DefaultMode,
		Namespace:        iConfig.Namespace,
		IgnoreNamespaces: iConfig.IgnoreNamespaces,
		APIPort:          iConfig.APIPort,
		APIHost:          iConfig.APIHost,
		MinTCPPort:       minTCPPort,
		MaxTCPPort:       minTCPPort + iConfig.LimitTCPPort,
		MinUDPPort:       minUDPPort,
		MaxUDPPort:       minUDPPort + iConfig.LimitUDPPort,
		MinHTTPPort:      minHTTPPort,
		MaxHTTPPort:      minHTTPPort + iConfig.LimitHTTPPort,
	}, log)
	if err != nil {
		return fmt.Errorf("unable to create controller: %w", err)
	}

	// run the ctr loop to process items
	if err = ctr.Run(stopCh); err != nil {
		log.Fatalf("Error running ctr: %v", err)
	}

	return nil
}
