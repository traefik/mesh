package prepare

import (
	"fmt"

	"github.com/containous/maesh/cmd"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/prepare"
	"github.com/containous/traefik/v2/pkg/cli"
)

// NewCmd builds a new Prepare command.
func NewCmd(pConfig *cmd.PrepareConfiguration, loaders []cli.ResourceLoader) *cli.Command {
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

func prepareCommand(pConfig *cmd.PrepareConfiguration) error {
	log, err := cmd.NewLogger(pConfig.LogFormat, pConfig.LogLevel, pConfig.Debug)
	if err != nil {
		return fmt.Errorf("could not build logger: %w", err)
	}

	log.Debugln("Starting maesh prepare...")
	log.Debugf("Using masterURL: %q", pConfig.MasterURL)
	log.Debugf("Using kubeconfig: %q", pConfig.KubeConfig)

	clients, err := k8s.NewClient(log, pConfig.MasterURL, pConfig.KubeConfig)
	if err != nil {
		return fmt.Errorf("error building clients: %v", err)
	}

	p := prepare.NewPrepare(log, clients)

	if pConfig.SMI {
		log.Warnf("SMI mode is deprecated, please consider using --acl instead")
	}

	aclEnabled := pConfig.ACL || pConfig.SMI

	log.Debugf("ACL mode enabled: %t", aclEnabled)

	if err = p.StartInformers(aclEnabled); err != nil {
		return fmt.Errorf("error during informer check: %v, this can be caused by pre-existing objects in your cluster that do not conform to the spec", err)
	}

	if err = p.ConfigureDNS(pConfig.ClusterDomain, pConfig.Namespace); err != nil {
		return fmt.Errorf("unable to configure DNS: %w", err)
	}

	return nil
}
