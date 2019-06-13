package k8s

import (
	"errors"
	"fmt"
	"strings"

	crdclientset "github.com/containous/traefik/pkg/provider/kubernetes/crd/generated/clientset/versioned"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ClientWrapper holds both the CRD and kube clients.
type ClientWrapper struct {
	CrdClient  *crdclientset.Clientset
	KubeClient *kubernetes.Clientset
}

// NewClientWrapper creates and returns both a kubernetes client, and a CRD client.
func NewClientWrapper(url string, kubeConfig string) (*ClientWrapper, error) {
	config, err := clientcmd.BuildConfigFromFlags(url, kubeConfig)
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

// InitCluster is used to initialize a kubernetes cluster with a variety of configuration options.
func (w *ClientWrapper) InitCluster() error {
	log.Infoln("Preparing Cluster...")

	log.Debugln("Creating mesh namespace...")
	if err := w.verifyNamespaceExists(MeshNamespace); err != nil {
		return err
	}

	log.Debugln("Patching CoreDNS...")
	if err := w.patchCoreDNS("coredns", metav1.NamespaceSystem); err != nil {
		return err
	}

	log.Infoln("Cluster Preparation Complete...")

	return nil
}

func (w *ClientWrapper) verifyNamespaceExists(namespace string) error {
	if _, err := w.KubeClient.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{}); err != nil {
		ns := &apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
			Spec: apiv1.NamespaceSpec{},
		}

		if _, err := w.KubeClient.CoreV1().Namespaces().Create(ns); err != nil {
			return fmt.Errorf("unable to create namespace %q: %v", namespace, err)
		}
		log.Infof("Namespace %q created successfully", namespace)
	} else {
		log.Debugf("Namespace %q already exist", namespace)
	}

	return nil
}

func (w *ClientWrapper) patchCoreDNS(deploymentName string, deploymentNamespace string) error {
	coreDeployment, err := w.KubeClient.AppsV1().Deployments(deploymentNamespace).Get(deploymentName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	log.Debugln("Patching CoreDNS configmap...")
	patched, err := w.patchCoreConfigMap(coreDeployment)
	if err != nil {
		return err
	}

	if !patched {
		log.Debugln("Restarting CoreDNS pods...")
		if err := w.restartCorePods(coreDeployment); err != nil {
			return err
		}
	}

	return nil
}

func (w *ClientWrapper) patchCoreConfigMap(coreDeployment *appsv1.Deployment) (bool, error) {
	var coreConfigMapName string
	if len(coreDeployment.Spec.Template.Spec.Volumes) == 0 {
		return false, errors.New("coreDNS configmap not defined")
	}

	coreConfigMapName = coreDeployment.Spec.Template.Spec.Volumes[0].ConfigMap.Name

	coreConfigMap, err := w.KubeClient.CoreV1().ConfigMaps(coreDeployment.Namespace).Get(coreConfigMapName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	if len(coreConfigMap.ObjectMeta.Labels) > 0 {
		if _, ok := coreConfigMap.ObjectMeta.Labels["traefik-mesh-patched"]; ok {
			log.Debugln("Configmap already patched...")
			return true, nil
		}
	}

	patchString := `loadbalance
    rewrite {
        name regex ([a-z]*)\.([a-z]*)\.traefik\.mesh traefik-{1}-{2}.traefik-mesh.svc.cluster.local
        answer name traefik-([a-z]*)-([a-z]*)\.traefik-mesh\.svc\.cluster\.local {1}.{2}.traefik.mesh
    }
`
	coreConfigMap.Data["Corefile"] = strings.Replace(coreConfigMap.Data["Corefile"], "loadbalance", patchString, 1)
	if len(coreConfigMap.ObjectMeta.Labels) == 0 {
		coreConfigMap.ObjectMeta.Labels = make(map[string]string)
	}
	coreConfigMap.ObjectMeta.Labels["traefik-mesh-patched"] = "true"

	if _, err = w.KubeClient.CoreV1().ConfigMaps(coreDeployment.Namespace).Update(coreConfigMap); err != nil {
		return false, err
	}

	return false, nil
}

func (w *ClientWrapper) restartCorePods(coreDeployment *appsv1.Deployment) error {
	log.Infoln("Restarting coreDNS pods...")

	//Never edit original object, always work with a clone for updates
	newDeployment := coreDeployment
	annotations := newDeployment.Spec.Template.Annotations
	if len(annotations) == 0 {
		annotations = make(map[string]string)
	}

	annotations["i3o-hash"] = uuid.New().String()
	newDeployment.Spec.Template.Annotations = annotations
	_, err := w.KubeClient.AppsV1().Deployments(newDeployment.Namespace).Update(newDeployment)

	return err
}

// VerifyCluster is used to verify a kubernetes cluster has been initialized properly.
func (w *ClientWrapper) VerifyCluster() error {
	log.Infoln("Verifying Cluster...")
	defer log.Infoln("Cluster Verification Complete...")

	log.Debugln("Verifying mesh namespace exists...")
	if err := w.verifyNamespaceExists(MeshNamespace); err != nil {
		return err
	}

	log.Debugln("Verifying CoreDNS Patched...")
	if err := w.verifyCoreDNSPatched("coredns", metav1.NamespaceSystem); err != nil {
		return err
	}

	return nil
}

func (w *ClientWrapper) verifyCoreDNSPatched(deploymentName string, namespace string) error {
	coreDeployment, err := w.KubeClient.AppsV1().Deployments(namespace).Get(deploymentName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if len(coreDeployment.Spec.Template.Spec.Volumes) == 0 {
		return errors.New("coreDNS configmap not defined")
	}

	coreConfigMapName := coreDeployment.Spec.Template.Spec.Volumes[0].ConfigMap.Name

	coreConfigmap, err := w.KubeClient.CoreV1().ConfigMaps(coreDeployment.Namespace).Get(coreConfigMapName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if len(coreConfigmap.ObjectMeta.Labels) > 0 {
		if _, ok := coreConfigmap.ObjectMeta.Labels["traefik-mesh-patched"]; ok {
			return nil
		}
	}

	return errors.New("coreDNS not patched. Run ./i3o patch to update DNS")
}

// buildClient returns a useable kubernetes client.
func buildKubernetesClient(config *rest.Config) (*kubernetes.Clientset, error) {
	log.Infoln("Building Kubernetes Client...")
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create kubernetes client: %v", err)
	}

	return client, nil
}

// buildKubernetesCRDClient returns a client to manage CRD objects.
func buildKubernetesCRDClient(config *rest.Config) (*crdclientset.Clientset, error) {
	log.Infoln("Building Kubernetes CRD Client...")
	client, err := crdclientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create CRD client: %v", err)
	}

	return client, nil
}
