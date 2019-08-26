package prepare

import (
	"fmt"
	"os"

	"github.com/containous/maesh/cmd"
	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/traefik/v2/pkg/cli"
	log "github.com/sirupsen/logrus"
)

// NewCmd builds a new Patch command.
func NewCmd(pConfig *cmd.PrepareConfig, loaders []cli.ResourceLoader) *cli.Command {
	return &cli.Command{
		Name:          "prepare",
		Description:   `Prepare command.`,
		Configuration: pConfig,
		Run: func(_ []string) error {
			return patchCommand(pConfig)
		},
		Resources: loaders,
	}
}

func patchCommand(pConfig *cmd.PrepareConfig) error {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
	if pConfig.Debug {
		log.SetLevel(log.DebugLevel)
	}

	log.Debugln("Starting maesh prepare...")
	log.Debugf("Using masterURL: %q", pConfig.MasterURL)
	log.Debugf("Using kubeconfig: %q", pConfig.KubeConfig)

	clients, err := k8s.NewClientWrapper(pConfig.MasterURL, pConfig.KubeConfig)
	if err != nil {
		return fmt.Errorf("error building clients: %v", err)
	}

	if err = clients.CheckCluster(); err != nil {
		return fmt.Errorf("error during cluster check: %v", err)
	}

	if err = clients.InitCluster(pConfig.Namespace); err != nil {
		return fmt.Errorf("error initializing cluster: %v", err)
	}

	return nil
}
