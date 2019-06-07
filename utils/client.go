package utils

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	crdclientset "github.com/containous/traefik/pkg/provider/kubernetes/crd/generated/clientset/versioned"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// ClientWrapper holds both the CRD and kube clients
type ClientWrapper struct {
	CrdClient  *crdclientset.Clientset
	KubeClient *kubernetes.Clientset
}

// buildClient returns a useable kubernetes client.
func buildKubernetesClient(config *rest.Config) (*kubernetes.Clientset, error) {
	log.Infoln("Building Kubernetes Client...")
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// buildKubernetesCRDClient returns a client to manage CRD objects.
func buildKubernetesCRDClient(config *rest.Config) (*crdclientset.Clientset, error) {
	log.Infoln("Building Kubernetes CRD Client...")
	client, err := crdclientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// BuildClients creates and returns both a kubernetes client, and a CRD client
func BuildClients(url string, kubeconfig, token, certauthfilepath string) (*ClientWrapper, error) {

	var config *rest.Config

	if url == "" && kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig := filepath.Join(home, ".kube", "config")
			log.Debugf("Looking for kubeConfig at %q", kubeconfig)
			// If no config is defined, see if there is a default kube config
			if _, err := os.Stat(kubeconfig); err == nil {
				config, err = clientcmd.BuildConfigFromFlags(url, kubeconfig)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	withEndpoint := ""
	if url != "" {
		withEndpoint = fmt.Sprintf(" with endpoint %v", url)
	}

	var err error
	if config == nil {
		if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
			log.Infof("Creating in-cluster client%s", withEndpoint)
			config, err = newInClusterClientConfig(url)
			if err != nil {
				return nil, err
			}
		} else {
			log.Infof("Creating cluster-external Provider client%s", withEndpoint)
			config, err = newExternalClusterClientConfig(url, token, certauthfilepath)
			if err != nil {
				return nil, err
			}
		}
	}

	kubeClient, err := buildKubernetesClient(config)
	if err != nil {
		return nil, err
	}

	crdClient, err := buildKubernetesCRDClient(config)
	if err != nil {
		return nil, err
	}

	return &ClientWrapper{
		CrdClient:  crdClient,
		KubeClient: kubeClient,
	}, nil
}

// newInClusterClient returns a new config to run inside the cluster.
func newInClusterClientConfig(endpoint string) (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster configuration: %s", err)
	}

	if endpoint != "" {
		config.Host = endpoint
	}

	return config, nil
}

// newExternalClusterClient returns a new config to run outside of the cluster.
// The endpoint parameter must not be empty.
func newExternalClusterClientConfig(endpoint, token, caFilePath string) (*rest.Config, error) {
	if endpoint == "" {
		return nil, errors.New("endpoint missing for external cluster client")
	}

	config := &rest.Config{
		Host:        endpoint,
		BearerToken: token,
	}

	if caFilePath != "" {
		caData, err := ioutil.ReadFile(caFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file %s: %s", caFilePath, err)
		}

		config.TLSClientConfig = rest.TLSClientConfig{CAData: caData}
	}

	return config, nil
}
