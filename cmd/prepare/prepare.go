package prepare

import (
	"fmt"
	"os"

	"github.com/containous/maesh/cmd"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/prepare"
	"github.com/containous/traefik/v2/pkg/cli"
	log "github.com/sirupsen/logrus"
)

// NewCmd builds a new Prepare command.
func NewCmd(pConfig *cmd.PrepareConfig, loaders []cli.ResourceLoader) *cli.Command {
	return &cli.Command{
		Name:          "prepare",
		Description:   `Prepare command.`,
		Configuration: pConfig,
		Run: func(_ []string) error {
			return prepareCommand(pConfig)
		},
		Resources: loaders,
	}
}

func prepareCommand(pConfig *cmd.PrepareConfig) error {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)

	if pConfig.Debug {
		log.SetLevel(log.DebugLevel)
	}

	log.Debugln("Starting maesh prepare...")
	log.Debugf("Using masterURL: %q", pConfig.MasterURL)
	log.Debugf("Using kubeconfig: %q", pConfig.KubeConfig)

	clients, err := k8s.NewClient(pConfig.MasterURL, pConfig.KubeConfig)
	if err != nil {
		return fmt.Errorf("error building clients: %v", err)
	}

	prepare := prepare.NewPrepare(clients)

	if err = prepare.CheckCluster(); err != nil {
		return fmt.Errorf("error during cluster check: %v", err)
	}

	if err = prepare.CheckInformersStart(pConfig.SMI); err != nil {
		return fmt.Errorf("error during informer check: %v, this can be caused by pre-existing objects in your cluster that do not conform to the spec", err)
	}

	if err = prepare.InitCluster(pConfig.Namespace, pConfig.ClusterDomain); err != nil {
		return fmt.Errorf("error initializing cluster: %v", err)
	}

	return nil
}
