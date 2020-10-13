package k8s

import (
	"fmt"
	"os"

	accessclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/clientset/versioned"
	specsclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/clientset/versioned"
	splitclient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/clientset/versioned"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client is an interface for the various resource controllers.
type Client interface {
	KubernetesClient() kubernetes.Interface
	AccessClient() accessclient.Interface
	SpecsClient() specsclient.Interface
	SplitClient() splitclient.Interface
}

// Ensure the client wrapper fits the Client interface.
var _ Client = (*ClientWrapper)(nil)

// ClientWrapper holds the clients for the various resource controllers.
type ClientWrapper struct {
	kubeClient   *kubernetes.Clientset
	accessClient *accessclient.Clientset
	specsClient  *specsclient.Clientset
	splitClient  *splitclient.Clientset
}

// NewClient creates and returns a ClientWrapper that satisfies the Client interface.
func NewClient(logger logrus.FieldLogger, masterURL, kubeConfig string) (Client, error) {
	config, err := buildConfig(logger, masterURL, kubeConfig)
	if err != nil {
		return nil, err
	}

	kubeClient, err := buildKubernetesClient(logger, config)
	if err != nil {
		return nil, err
	}

	accessClient, err := buildSmiAccessClient(logger, config)
	if err != nil {
		return nil, err
	}

	specsClient, err := buildSmiSpecsClient(logger, config)
	if err != nil {
		return nil, err
	}

	splitClient, err := buildSmiSplitClient(logger, config)
	if err != nil {
		return nil, err
	}

	return &ClientWrapper{
		kubeClient:   kubeClient,
		accessClient: accessClient,
		specsClient:  specsClient,
		splitClient:  splitClient,
	}, nil
}

// buildConfig takes the master URL and kubeconfig, and returns an external or internal config.
func buildConfig(logger logrus.FieldLogger, masterURL, kubeConfig string) (*rest.Config, error) {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
		// If these env vars are set, we can build an in-cluster config.
		logger.Debug("Creating in-cluster client")
		return rest.InClusterConfig()
	}

	if masterURL != "" || kubeConfig != "" {
		logger.Debug("Creating cluster-external client from provided masterURL or kubeconfig")
		return clientcmd.BuildConfigFromFlags(masterURL, kubeConfig)
	}

	return nil, fmt.Errorf("could not create client: missing masterURL or kubeConfig")
}

// KubernetesClient is used to get the kubernetes clientset.
func (w *ClientWrapper) KubernetesClient() kubernetes.Interface {
	return w.kubeClient
}

// AccessClient is used to get the SMI Access clientset.
func (w *ClientWrapper) AccessClient() accessclient.Interface {
	return w.accessClient
}

// SpecsClient is used to get the SMI Specs clientset.
func (w *ClientWrapper) SpecsClient() specsclient.Interface {
	return w.specsClient
}

// SplitClient is used to get the SMI Split clientset.
func (w *ClientWrapper) SplitClient() splitclient.Interface {
	return w.splitClient
}

// buildClient returns a useable kubernetes client.
func buildKubernetesClient(logger logrus.FieldLogger, config *rest.Config) (*kubernetes.Clientset, error) {
	logger.Debug("Building Kubernetes Client...")

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create kubernetes client: %v", err)
	}

	return client, nil
}

// buildSmiAccessClient returns a client to manage SMI Access objects.
func buildSmiAccessClient(logger logrus.FieldLogger, config *rest.Config) (*accessclient.Clientset, error) {
	logger.Debug("Building SMI Access Client...")

	client, err := accessclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Access Client: %v", err)
	}

	return client, nil
}

// buildSmiSpecsClient returns a client to manage SMI Specs objects.
func buildSmiSpecsClient(logger logrus.FieldLogger, config *rest.Config) (*specsclient.Clientset, error) {
	logger.Debug("Building SMI Specs Client...")

	client, err := specsclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Specs Client: %v", err)
	}

	return client, nil
}

// buildSmiSplitClient returns a client to manage SMI Split objects.
func buildSmiSplitClient(logger logrus.FieldLogger, config *rest.Config) (*splitclient.Clientset, error) {
	logger.Debug("Building SMI Split Client...")

	client, err := splitclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Split Client: %v", err)
	}

	return client, nil
}
