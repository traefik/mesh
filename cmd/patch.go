package cmd

import (
	"os"

	"github.com/containous/i3o/internal/k8s"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// patchCmd represents the patch command.
var patchCmd = &cobra.Command{
	Use:   "patch",
	Short: "Patch cluster",
	Run:   patchCommand(),
}

func init() {
	patchCmd.Flags().StringVar(&kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "Path to a kubeconfig. Only required if out-of-cluster.")
	patchCmd.Flags().StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	patchCmd.Flags().BoolVar(&debug, "debug", false, "enable debug mode")
	rootCmd.Flags().BoolVar(&smi, "smi", false, "enable SMI")
	rootCmd.AddCommand(patchCmd)
}

func patchCommand() func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		log.SetOutput(os.Stdout)
		log.SetLevel(log.InfoLevel)
		if debug {
			log.SetLevel(log.DebugLevel)
		}

		log.Debugln("Starting i3o patch...")
		log.Debugf("Using masterURL: %q", masterURL)
		log.Debugf("Using kubeconfig: %q", kubeconfig)

		clients, err := k8s.NewClientWrapper(masterURL, kubeconfig)
		if err != nil {
			log.Fatalf("Error building clients: %v", err)
		}

		if err = clients.InitCluster(smi); err != nil {
			log.Fatalf("Error initializing cluster: %v", err)
		}

	}
}
