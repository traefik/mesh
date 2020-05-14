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
	GetKubernetesClient() kubernetes.Interface
	GetAccessClient() accessclient.Interface
	GetSpecsClient() specsclient.Interface
	GetSplitClient() splitclient.Interface
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
func NewClient(log logrus.FieldLogger, url string, kubeConfig string) (Client, error) {
	config, err := buildConfig(log, url, kubeConfig)
	if err != nil {
		return nil, err
	}

	kubeClient, err := buildKubernetesClient(log, config)
	if err != nil {
		return nil, err
	}

	accessClient, err := buildSmiAccessClient(log, config)
	if err != nil {
		return nil, err
	}

	specsClient, err := buildSmiSpecsClient(log, config)
	if err != nil {
		return nil, err
	}

	splitClient, err := buildSmiSplitClient(log, config)
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

// buildConfig takes the url and kubeconfig, and returns an external or internal config.
func buildConfig(log logrus.FieldLogger, url string, kubeConfig string) (*rest.Config, error) {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
		// If these env vars are set, we can build an in-cluster config.
		log.Infoln("Creating in-cluster client")
		return rest.InClusterConfig()
	}

	if url != "" && kubeConfig != "" {
		log.Infoln("Creating cluster-external client from masterURL and kubeconfig")
		return clientcmd.BuildConfigFromFlags(url, kubeConfig)
	}

	return nil, fmt.Errorf("Could not create client: Missing masterURL or kubeConfig")
}

// GetKubernetesClient is used to get the kubernetes clientset.
func (w *ClientWrapper) GetKubernetesClient() kubernetes.Interface {
	return w.kubeClient
}

// GetAccessClient is used to get the SMI Access clientset.
func (w *ClientWrapper) GetAccessClient() accessclient.Interface {
	return w.accessClient
}

// GetSpecsClient is used to get the SMI Specs clientset.
func (w *ClientWrapper) GetSpecsClient() specsclient.Interface {
	return w.specsClient
}

// GetSplitClient is used to get the SMI Split clientset.
func (w *ClientWrapper) GetSplitClient() splitclient.Interface {
	return w.splitClient
}

// buildClient returns a useable kubernetes client.
func buildKubernetesClient(log logrus.FieldLogger, config *rest.Config) (*kubernetes.Clientset, error) {
	log.Debugln("Building Kubernetes Client...")

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create kubernetes client: %v", err)
	}

	return client, nil
}

// buildSmiAccessClient returns a client to manage SMI Access objects.
func buildSmiAccessClient(log logrus.FieldLogger, config *rest.Config) (*accessclient.Clientset, error) {
	log.Debugln("Building SMI Access Client...")

	client, err := accessclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Access Client: %v", err)
	}

	return client, nil
}

// buildSmiSpecsClient returns a client to manage SMI Specs objects.
func buildSmiSpecsClient(log logrus.FieldLogger, config *rest.Config) (*specsclient.Clientset, error) {
	log.Debugln("Building SMI Specs Client...")

	client, err := specsclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Specs Client: %v", err)
	}

	return client, nil
}

// buildSmiSplitClient returns a client to manage SMI Split objects.
func buildSmiSplitClient(log logrus.FieldLogger, config *rest.Config) (*splitclient.Clientset, error) {
	log.Debugln("Building SMI Split Client...")

	client, err := splitclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Split Client: %v", err)
	}

	return client, nil
}
