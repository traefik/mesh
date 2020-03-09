package k8s

import (
	"fmt"

	accessClient "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/clientset/versioned"
	specsClient "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/clientset/versioned"
	splitClient "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/clientset/versioned"
	log "github.com/sirupsen/logrus"
	kubeerror "k8s.io/apimachinery/pkg/api/errors"
	kubeClient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client is an interface for the various resource controllers.
type Client interface {
	GetKubernetesClient() kubeClient.Interface
	GetAccessClient() accessClient.Interface
	GetSpecsClient() specsClient.Interface
	GetSplitClient() splitClient.Interface
}

// Ensure the client wrapper fits the Client interface
var _ Client = (*ClientWrapper)(nil)

// ClientWrapper holds the clients for the various resource controllers.
type ClientWrapper struct {
	kubeClient   *kubeClient.Clientset
	accessClient *accessClient.Clientset
	specsClient  *specsClient.Clientset
	splitClient  *splitClient.Clientset
}

// NewClient creates and returns a ClientWrapper that satisfies the Client interface.
func NewClient(url string, kubeConfig string) (Client, error) {
	config, err := clientcmd.BuildConfigFromFlags(url, kubeConfig)
	if err != nil {
		return nil, err
	}

	kubeClient, err := buildKubernetesClient(config)
	if err != nil {
		return nil, err
	}

	accessClient, err := buildSmiAccessClient(config)
	if err != nil {
		return nil, err
	}

	specsClient, err := buildSmiSpecsClient(config)
	if err != nil {
		return nil, err
	}

	splitClient, err := buildSmiSplitClient(config)
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

// GetKubernetesClient is used to get the kubernetes clientset.
func (w *ClientWrapper) GetKubernetesClient() kubeClient.Interface {
	return w.kubeClient
}

// GetAccessClient is used to get the SMI Access clientset.
func (w *ClientWrapper) GetAccessClient() accessClient.Interface {
	return w.accessClient
}

// GetSpecsClient is used to get the SMI Specs clientset.
func (w *ClientWrapper) GetSpecsClient() specsClient.Interface {
	return w.specsClient
}

// GetSplitClient is used to get the SMI Split clientset.
func (w *ClientWrapper) GetSplitClient() splitClient.Interface {
	return w.splitClient
}

// buildClient returns a useable kubernetes client.
func buildKubernetesClient(config *rest.Config) (*kubeClient.Clientset, error) {
	log.Debugln("Building Kubernetes Client...")

	client, err := kubeClient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create kubernetes client: %v", err)
	}

	return client, nil
}

// buildSmiAccessClient returns a client to manage SMI Access objects.
func buildSmiAccessClient(config *rest.Config) (*accessClient.Clientset, error) {
	log.Debugln("Building SMI Access Client...")

	client, err := accessClient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Access Client: %v", err)
	}

	return client, nil
}

// buildSmiSpecsClient returns a client to manage SMI Specs objects.
func buildSmiSpecsClient(config *rest.Config) (*specsClient.Clientset, error) {
	log.Debugln("Building SMI Specs Client...")

	client, err := specsClient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Specs Client: %v", err)
	}

	return client, nil
}

// buildSmiSplitClient returns a client to manage SMI Split objects.
func buildSmiSplitClient(config *rest.Config) (*splitClient.Clientset, error) {
	log.Debugln("Building SMI Split Client...")

	client, err := splitClient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Split Client: %v", err)
	}

	return client, nil
}

// TranslateNotFoundError will translate a "not found" error to a boolean return
// value which indicates if the resource exists and a nil error.
func TranslateNotFoundError(err error) (bool, error) {
	if kubeerror.IsNotFound(err) {
		return false, nil
	}

	return err == nil, err
}
