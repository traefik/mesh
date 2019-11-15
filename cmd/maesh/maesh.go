package main

import (
	"context"
	"fmt"
	stdlog "log"
	"os"
	"time"

	"github.com/containous/maesh/cmd"
	"github.com/containous/maesh/cmd/prepare"
	"github.com/containous/maesh/cmd/version"
	"github.com/containous/maesh/internal/configurator"
	"github.com/containous/maesh/internal/deployer"
	"github.com/containous/maesh/internal/mesher"
	kprovider "github.com/containous/maesh/internal/providers/kubernetes"
	"github.com/containous/maesh/internal/signals"
	"github.com/containous/traefik/v2/pkg/cli"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
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

func maeshCommand(cfg *cmd.MaeshConfiguration) error {
	ctx := context.Background()

	logger := log.New().WithFields(log.Fields{"app": "maesh", "component": "controller"})
	logger.Info("Starting Maesh controller")

	kcfg, err := clientcmd.BuildConfigFromFlags(cfg.MasterURL, cfg.KubeConfig)
	if err != nil {
		return err
	}

	clientSet, err := kubernetes.NewForConfig(kcfg)
	if err != nil {
		return fmt.Errorf("unable to create kubernetes client: %v", err)
	}

	informerFactory := informers.NewSharedInformerFactoryWithOptions(
		clientSet,
		30*time.Second,
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.FieldSelector = "metadata.namespace!=kube-system,metadata.name!=kubernetes"
		}),
	)

	mesher := mesher.NewController(clientSet, informerFactory, cfg.Namespace, logger)

	provider := kprovider.New(
		informerFactory.Core().V1().Services().Lister(),
		informerFactory.Core().V1().Endpoints().Lister(),
		cfg.DefaultMode,
	)

	deployer := deployer.NewREST(logger)

	configurator := configurator.NewController(informerFactory, provider, deployer, cfg.Namespace, logger)

	go func() {
		<-signals.SetupSignalHandler()
		logger.Info("Received signal, stopping...")
		mesher.ShutDown()
		configurator.ShutDown()
	}()

	informerFactory.Start(ctx.Done())
	informerFactory.WaitForCacheSync(ctx.Done())

	go mesher.Run()
	go configurator.Run()

	if err := mesher.Wait(ctx); err != nil {
		return err
	}
	if err := configurator.Wait(ctx); err != nil {
		return err
	}

	logger.Info("Maesh controller stopped")

	return nil
}
