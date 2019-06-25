package patch

import (
	"fmt"
	"os"

	"github.com/containous/i3o/cmd"
	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/traefik/pkg/cli"
	log "github.com/sirupsen/logrus"
)

// NewCmd builds a new Version command
func NewCmd(pConfig *cmd.PatchConfig, loaders []cli.ResourceLoader) *cli.Command {
	return &cli.Command{
		Name:          "patch",
		Description:   `Patch command.`,
		Configuration: pConfig,
		Run: func(_ []string) error {
			return patchCommand(pConfig)
		},
		Resources: loaders,
	}
}

func patchCommand(pConfig *cmd.PatchConfig) error {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
	if pConfig.Debug {
		log.SetLevel(log.DebugLevel)
	}

	log.Debugln("Starting i3o patch...")
	log.Debugf("Using masterURL: %q", pConfig.MasterURL)
	log.Debugf("Using kubeconfig: %q", pConfig.KubeConfig)

	clients, err := k8s.NewClientWrapper(pConfig.MasterURL, pConfig.KubeConfig)
	if err != nil {
		return fmt.Errorf("error building clients: %v", err)
	}

	if err = clients.InitCluster(); err != nil {
		return fmt.Errorf("error initializing cluster: %v", err)
	}
	return nil
}
