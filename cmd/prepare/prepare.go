package prepare

import (
	"fmt"
	"os"

	"github.com/containous/maesh/cmd"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/prepare"
	"github.com/containous/traefik/v2/pkg/cli"
	"github.com/sirupsen/logrus"
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
	log := logrus.New()

	log.SetOutput(os.Stdout)

	logLevelStr := pConfig.LogLevel
	if pConfig.Debug {
		logLevelStr = "debug"

		log.Warnf("Debug flag is deprecated, please consider using --loglevel=DEBUG instead")
	}

	logLevel, err := logrus.ParseLevel(logLevelStr)
	if err != nil {
		return err
	}

	log.SetLevel(logLevel)

	// configure log format
	var formatter logrus.Formatter
	if pConfig.LogFormat == "json" {
		formatter = &logrus.JSONFormatter{}
	} else {
		formatter = &logrus.TextFormatter{DisableColors: false, FullTimestamp: true, DisableSorting: true}
	}

	log.SetFormatter(formatter)

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
