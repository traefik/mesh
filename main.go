package main

import (
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/containous/i3o/meshcontroller"
	"github.com/containous/i3o/utils"
	"k8s.io/client-go/util/homedir"
)

var demo bool
var kubeconfig string
var debug bool

func init() {
	flag.BoolVar(&demo, "demo", false, "install demo data")
	flag.BoolVar(&debug, "debug", false, "enable debug mode")

	if home := homedir.HomeDir(); home != "" {
		flag.StringVar(&kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	}

	flag.Parse()

	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
	if debug {
		log.SetLevel(log.DebugLevel)
	}
}

func main() {
	client, err := utils.BuildClient(kubeconfig)
	if err != nil {
		panic(err)
	}

	if err = utils.InitCluster(client, demo); err != nil {
		panic(err)
	}

	var meshConfig *utils.TraefikMeshConfig
	if meshConfig, err = utils.CreateMeshConfig(client); err != nil {
		panic(err)
	}

	if err := utils.CreateRoutingConfigmap(client, meshConfig); err != nil {
		panic(err)
	}

	// Create a new controller.
	controller := meshcontroller.NewMeshController()

	// Initialize the controller.
	controller.Init(client)

	// use a channel to synchronize the finalization for a graceful shutdown
	stopCh := make(chan struct{})
	defer close(stopCh)

	// run the controller loop to process items
	go controller.Run(stopCh)

	// use a channel to handle OS signals to terminate and gracefully shut
	// down processing
	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	<-sigTerm

}
