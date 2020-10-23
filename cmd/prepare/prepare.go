package prepare

import (
	"fmt"

	"github.com/traefik/mesh/v2/cmd"
	"github.com/traefik/mesh/v2/pkg/k8s"
	"github.com/traefik/paerser/cli"
)

// NewCmd builds a new Prepare command.
func NewCmd(config *Configuration, loaders []cli.ResourceLoader) *cli.Command {
	return &cli.Command{
		Name:          "prepare",
		Description:   `Prepare command.`,
		Configuration: config,
		Run: func(_ []string) error {
			return prepareCommand(config)
		},
		Resources: loaders,
	}
}

func prepareCommand(config *Configuration) error {
	logger, err := cmd.NewLogger(config.LogFormat, config.LogLevel)
	if err != nil {
		return fmt.Errorf("could not create logger: %w", err)
	}

	logger.Debug("Starting prepare...")
	logger.Debugf("Using masterURL: %q", config.MasterURL)
	logger.Debugf("Using kubeconfig: %q", config.KubeConfig)
	logger.Debugf("ACL mode enabled: %t", config.ACL)

	clients, err := k8s.NewClient(logger, config.MasterURL, config.KubeConfig)
	if err != nil {
		return fmt.Errorf("error building clients: %w", err)
	}

	if err = k8s.CheckSMIVersion(clients.KubernetesClient(), config.ACL); err != nil {
		return fmt.Errorf("unsupported SMI version: %w", err)
	}

	return nil
}
