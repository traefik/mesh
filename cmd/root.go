package cmd

import (
	"os"

	"github.com/containous/i3o/meshcontroller"
	"github.com/containous/i3o/utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
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
	rootCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	rootCmd.Flags().StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	rootCmd.Flags().BoolVar(&demo, "demo", false, "install demo data")
	rootCmd.Flags().BoolVar(&debug, "debug", false, "enable debug mode")
}

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:     "i3o",
	Short:   "i3o controller",
	Long:    "i3o controller",
	Version: version,
	Run:     runCommand(),
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

func runCommand() func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
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

		if err = utils.CreateRoutingConfigmap(kubeClient, meshConfig); err != nil {
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
}
