package main

import (
	"flag"
	"os"

	"github.com/containous/i3o/meshcontroller"
	"github.com/containous/i3o/utils"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/sample-controller/pkg/signals"
)

var (
	demo       bool
	debug      bool
	kubeconfig string
	masterURL  string
)

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.BoolVar(&demo, "demo", false, "install demo data")
	flag.BoolVar(&debug, "debug", false, "enable debug mode")
}

func main() {
	flag.Parse()

	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
	if debug {
		log.SetLevel(log.DebugLevel)
	}

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		log.Fatalf("Error building kubeconfig: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Error building kubernetes clientset: %v", err)
	}

	if err = utils.InitCluster(kubeClient, demo); err != nil {
		log.Fatalf("Error initializing cluster: %v", err)
	}

	var meshConfig *utils.TraefikMeshConfig
	if meshConfig, err = utils.CreateMeshConfig(kubeClient); err != nil {
		log.Fatalf("Error creating mesh config: %v", err)
	}

	if err := utils.CreateRoutingConfigmap(kubeClient, meshConfig); err != nil {
		log.Fatalf("Error creating routing config map: %v", err)
	}

	// Create a new controller.
	controller := meshcontroller.NewMeshController()

	// Initialize the controller.
	controller.Init(kubeClient)

	// run the controller loop to process items
	if err = controller.Run(stopCh); err != nil {
		log.Fatalf("Error running controller: %v", err)
	}
}
