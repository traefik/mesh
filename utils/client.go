package utils

import (
	crdclientset "github.com/containous/traefik/pkg/provider/kubernetes/crd/generated/clientset/versioned"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
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
func BuildClients(url string, kubeconfig string) (*ClientWrapper, error) {
	config, err := clientcmd.BuildConfigFromFlags(url, kubeconfig)
	if err != nil {
		return nil, err
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
