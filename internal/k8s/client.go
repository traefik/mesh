package k8s

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	smiAccessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	smiSpecsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	smiAccessClientset "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/clientset/versioned"
	smiSpecsClientset "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/clientset/versioned"
	smiSplitClientset "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/clientset/versioned"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kubeerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	supportedCoreDNSVersions = []string{
		"1.3",
		"1.4",
	}
)

// Client is an interface that represents a full-featured kubernetes client wrapper
type Client interface {
	CoreV1Client
	AppsV1Client
	SMIClient
}

// ClusterInitClient is an interface that can be used for doing cluster initialization.
type ClusterInitClient interface {
	InitCluster() error
	VerifyCluster() error
}

type CoreV1Client interface {
	GetService(namespace, name string) (*corev1.Service, bool, error)
	GetServices(namespace string) ([]*corev1.Service, error)
	ListServicesWithOptions(namespace string, options metav1.ListOptions) (*corev1.ServiceList, error)
	WatchServicesWithOptions(namespace string, options metav1.ListOptions) (watch.Interface, error)
	DeleteService(namespace, name string) error
	CreateService(service *corev1.Service) (*corev1.Service, error)
	UpdateService(service *corev1.Service) (*corev1.Service, error)
	GetEndpoints(namespace, name string) (*corev1.Endpoints, bool, error)
	GetPod(namespace, name string) (*corev1.Pod, bool, error)
	ListPodWithOptions(namespace string, options metav1.ListOptions) (*corev1.PodList, error)
	GetNamespace(name string) (*corev1.Namespace, bool, error)
	GetNamespaces() ([]*corev1.Namespace, error)
	GetConfigMap(namespace, name string) (*corev1.ConfigMap, bool, error)
	UpdateConfigMap(configMap *corev1.ConfigMap) (*corev1.ConfigMap, error)
	CreateConfigMap(configMap *corev1.ConfigMap) (*corev1.ConfigMap, error)
}

type AppsV1Client interface {
	GetDeployment(namespace, name string) (*appsv1.Deployment, bool, error)
	UpdateDeployment(deployment *appsv1.Deployment) (*appsv1.Deployment, error)
}

type SMIClient interface {
	SMIAccessV1Alpha1Client
	SMISpecsV1Alpha1Client
	SMISplitV1Alpha1Client
}

type SMIAccessV1Alpha1Client interface {
	GetTrafficTargets() ([]*smiAccessv1alpha1.TrafficTarget, error)
}

type SMISpecsV1Alpha1Client interface {
	GetHTTPRouteGroup(namespace, name string) (*smiSpecsv1alpha1.HTTPRouteGroup, bool, error)
}

type SMISplitV1Alpha1Client interface {
}

// ClientWrapper holds the clients for the various resource controllers.
type ClientWrapper struct {
	KubeClient      *kubernetes.Clientset
	SmiAccessClient *smiAccessClientset.Clientset
	SmiSpecsClient  *smiSpecsClientset.Clientset
	SmiSplitClient  *smiSplitClientset.Clientset
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

	smiAccessClient, err := buildSmiAccessClient(config)
	if err != nil {
		return nil, err
	}

	smiSpecsClient, err := buildSmiSpecsClient(config)
	if err != nil {
		return nil, err
	}

	smiSplitClient, err := buildSmiSplitClient(config)
	if err != nil {
		return nil, err
	}

	return &ClientWrapper{
		KubeClient:      kubeClient,
		SmiAccessClient: smiAccessClient,
		SmiSpecsClient:  smiSpecsClient,
		SmiSplitClient:  smiSplitClient,
	}, nil
}

// CheckCluster is used to check the cluster.
func (w *ClientWrapper) CheckCluster() error {
	log.Infoln("Checking Cluster...")

	log.Debugln("Creating CoreDNS version...")
	deployment, exists, err := w.GetDeployment(metav1.NamespaceSystem, "coredns")
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("%s does not exist in namespace %s", "coredns", metav1.NamespaceSystem)
	}

	var version string

	for _, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name != "coredns" {
			continue
		}

		split := strings.Split(c.Image, ":")
		if len(split) == 2 {
			version = split[1]
		}
	}

	if !isCoreDNSVersionSupported(version) {
		return fmt.Errorf("unsupported CoreDNS version %q, (supported versions are: %s)", version, strings.Join(supportedCoreDNSVersions, ","))
	}

	log.Infoln("Cluster check Complete...")

	return nil
}

// isCoreDNSVersionSupported returns true if the provided string contains a supported CoreDNS version.
func isCoreDNSVersionSupported(versionLine string) bool {
	for _, v := range supportedCoreDNSVersions {
		if strings.Contains(versionLine, v) || strings.Contains(versionLine, "v"+v) {
			return true
		}
	}

	return false
}

// InitCluster is used to initialize a kubernetes cluster with a variety of configuration options.
func (w *ClientWrapper) InitCluster(namespace string) error {
	log.Infoln("Preparing Cluster...")

	log.Debugln("Patching CoreDNS...")
	if err := w.patchCoreDNS("coredns", metav1.NamespaceSystem); err != nil {
		return err
	}

	log.Debugln("Creating TCP State Table...")
	if err := w.createTCPStateTable(namespace); err != nil {
		return err
	}
	log.Infoln("Cluster Preparation Complete...")

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

func (w *ClientWrapper) createTCPStateTable(namespace string) error {
	_, exists, err := w.GetConfigMap(namespace, TCPStateConfigmapName)
	if err != nil {
		return err
	}

	if !exists {
		_, err := w.CreateConfigMap(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      TCPStateConfigmapName,
				Namespace: namespace,
			},
		})
		return err
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
		if _, ok := coreConfigMap.ObjectMeta.Labels["maesh-patched"]; ok {
			log.Debugln("Configmap already patched...")
			return true, nil
		}
	}

	serverBlock :=
		`
maesh:53 {
	kubernetes cluster.local
	k8s_external maesh
}
`
	originalBlock := coreConfigMap.Data["Corefile"]
	newBlock := originalBlock + serverBlock
	coreConfigMap.Data["Corefile"] = newBlock
	if len(coreConfigMap.ObjectMeta.Labels) == 0 {
		coreConfigMap.ObjectMeta.Labels = make(map[string]string)
	}
	coreConfigMap.ObjectMeta.Labels["maesh-patched"] = "true"

	if _, err = w.KubeClient.CoreV1().ConfigMaps(coreDeployment.Namespace).Update(coreConfigMap); err != nil {
		return false, err
	}

	return false, nil
}

func (w *ClientWrapper) restartCorePods(coreDeployment *appsv1.Deployment) error {
	log.Infoln("Restarting coreDNS pods...")

	// Never edit original object, always work with a clone for updates.
	newDeployment := coreDeployment.DeepCopy()
	annotations := newDeployment.Spec.Template.Annotations
	if len(annotations) == 0 {
		annotations = make(map[string]string)
	}

	annotations["maesh-hash"] = uuid.New().String()
	newDeployment.Spec.Template.Annotations = annotations
	_, err := w.KubeClient.AppsV1().Deployments(newDeployment.Namespace).Update(newDeployment)

	return err
}

// VerifyCluster is used to verify a kubernetes cluster has been initialized properly.
func (w *ClientWrapper) VerifyCluster() error {
	log.Infoln("Verifying Cluster...")
	defer log.Infoln("Cluster Verification Complete...")

	log.Debugln("Verifying CoreDNS Patched...")
	if err := w.isCoreDNSPatched("coredns", metav1.NamespaceSystem); err != nil {
		return err
	}

	return nil
}

func (w *ClientWrapper) isCoreDNSPatched(deploymentName string, namespace string) error {
	coreDeployment, err := w.KubeClient.AppsV1().Deployments(namespace).Get(deploymentName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if len(coreDeployment.Spec.Template.Spec.Volumes) == 0 {
		return errors.New("coreDNS configmap not defined")
	}

	coreConfigMapName := coreDeployment.Spec.Template.Spec.Volumes[0].ConfigMap.Name

	coreConfigMap, err := w.KubeClient.CoreV1().ConfigMaps(coreDeployment.Namespace).Get(coreConfigMapName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if len(coreConfigMap.ObjectMeta.Labels) > 0 {
		if _, ok := coreConfigMap.ObjectMeta.Labels["maesh-patched"]; ok {
			return nil
		}
	}

	return errors.New("coreDNS not patched. Run ./maesh patch to update DNS")
}

// buildClient returns a useable kubernetes client.
func buildKubernetesClient(config *rest.Config) (*kubernetes.Clientset, error) {
	log.Debugln("Building Kubernetes Client...")
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create kubernetes client: %v", err)
	}

	return client, nil
}

// buildSmiAccessClient returns a client to manage SMI Access objects.
func buildSmiAccessClient(config *rest.Config) (*smiAccessClientset.Clientset, error) {
	log.Debugln("Building SMI Access Client...")
	client, err := smiAccessClientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Access Client: %v", err)
	}

	return client, nil
}

// buildSmiSpecsClient returns a client to manage SMI Specs objects.
func buildSmiSpecsClient(config *rest.Config) (*smiSpecsClientset.Clientset, error) {
	log.Debugln("Building SMI Specs Client...")
	client, err := smiSpecsClientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Specs Client: %v", err)
	}

	return client, nil
}

// buildSmiSplitClient returns a client to manage SMI Split objects.
func buildSmiSplitClient(config *rest.Config) (*smiSplitClientset.Clientset, error) {
	log.Debugln("Building SMI Split Client...")
	client, err := smiSplitClientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Split Client: %v", err)
	}

	return client, nil
}

// GetService retrieves the service from the specified namespace.
func (w *ClientWrapper) GetService(namespace, name string) (*corev1.Service, bool, error) {
	service, err := w.KubeClient.CoreV1().Services(namespace).Get(name, metav1.GetOptions{})
	exists, err := translateNotFoundError(err)
	return service, exists, err
}

// GetServices retrieves the services from the specified namespace.
func (w *ClientWrapper) GetServices(namespace string) ([]*corev1.Service, error) {
	var result []*corev1.Service
	list, err := w.KubeClient.CoreV1().Services(namespace).List(metav1.ListOptions{})
	if err != nil {
		return result, err
	}
	for _, service := range list.Items {
		result = append(result, &service)
	}
	return result, nil
}

// DeleteService deletes the service from the specified namespace.
func (w *ClientWrapper) DeleteService(namespace, name string) error {
	return w.KubeClient.CoreV1().Services(namespace).Delete(name, &metav1.DeleteOptions{})
}

// CreateService create the specified service.
func (w *ClientWrapper) CreateService(service *corev1.Service) (*corev1.Service, error) {
	return w.KubeClient.CoreV1().Services(service.Namespace).Create(service)
}

// UpdateService updates the specified service.
func (w *ClientWrapper) UpdateService(service *corev1.Service) (*corev1.Service, error) {
	return w.KubeClient.CoreV1().Services(service.Namespace).Update(service)
}

// ListServicesWithOptions lists services with the specified options.
func (w *ClientWrapper) ListServicesWithOptions(namespace string, options metav1.ListOptions) (*corev1.ServiceList, error) {
	return w.KubeClient.CoreV1().Services(namespace).List(options)
}

// WatchServicesWithOptions watches services with the specified options.
func (w *ClientWrapper) WatchServicesWithOptions(namespace string, options metav1.ListOptions) (watch.Interface, error) {
	return w.KubeClient.CoreV1().Services(namespace).Watch(options)
}

// GetEndpoints retrieves the endpoints from the specified namespace.
func (w *ClientWrapper) GetEndpoints(namespace, name string) (*corev1.Endpoints, bool, error) {
	endpoints, err := w.KubeClient.CoreV1().Endpoints(namespace).Get(name, metav1.GetOptions{})
	exists, err := translateNotFoundError(err)
	return endpoints, exists, err
}

// GetPod retrieves the pod from the specified namespace.
func (w *ClientWrapper) GetPod(namespace, name string) (*corev1.Pod, bool, error) {
	pod, err := w.KubeClient.CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
	exists, err := translateNotFoundError(err)
	return pod, exists, err
}

func (w *ClientWrapper) ListPodWithOptions(namespace string, options metav1.ListOptions) (*corev1.PodList, error) {
	return w.KubeClient.CoreV1().Pods(namespace).List(options)
}

// GetNamespace returns a namespace.
func (w *ClientWrapper) GetNamespace(name string) (*corev1.Namespace, bool, error) {
	pod, err := w.KubeClient.CoreV1().Namespaces().Get(name, metav1.GetOptions{})
	exists, err := translateNotFoundError(err)
	return pod, exists, err
}

// GetNamespaces returns a slice of all namespaces.
func (w *ClientWrapper) GetNamespaces() ([]*corev1.Namespace, error) {
	var result []*corev1.Namespace
	list, err := w.KubeClient.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return result, err
	}
	for _, namespace := range list.Items {
		result = append(result, &namespace)
	}
	return result, nil
}

// CreateNamespace creates a namespace if it doesn't exist.
func (w *ClientWrapper) CreateNamespace(namespace string) error {
	if _, err := w.KubeClient.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{}); err != nil {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
			Spec: corev1.NamespaceSpec{},
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

// GetDeployment retrieves the deployment from the specified namespace.
func (w *ClientWrapper) GetDeployment(namespace, name string) (*appsv1.Deployment, bool, error) {
	deployment, err := w.KubeClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	exists, err := translateNotFoundError(err)
	return deployment, exists, err
}

// UpdateDeployment updates the specified deployment.
func (w *ClientWrapper) UpdateDeployment(deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	return w.KubeClient.AppsV1().Deployments(deployment.Namespace).Update(deployment)
}

// GetTrafficTargets returns a slice of all TrafficTargets.
func (w *ClientWrapper) GetTrafficTargets() ([]*smiAccessv1alpha1.TrafficTarget, error) {
	var result []*smiAccessv1alpha1.TrafficTarget
	list, err := w.SmiAccessClient.AccessV1alpha1().TrafficTargets(metav1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return result, err
	}
	for _, trafficTarget := range list.Items {
		t := trafficTarget.DeepCopy()
		result = append(result, t)
	}
	return result, nil
}

// GetHTTPRouteGroup retrieves the HTTPRouteGroup from the specified namespace.
func (w *ClientWrapper) GetHTTPRouteGroup(namespace, name string) (*smiSpecsv1alpha1.HTTPRouteGroup, bool, error) {
	group, err := w.SmiSpecsClient.SpecsV1alpha1().HTTPRouteGroups(namespace).Get(name, metav1.GetOptions{})
	exists, err := translateNotFoundError(err)
	return group, exists, err
}

// GetConfigMap retrieves the named configMap in the specified namespace.
func (w *ClientWrapper) GetConfigMap(namespace, name string) (*corev1.ConfigMap, bool, error) {
	configMap, err := w.KubeClient.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
	exists, err := translateNotFoundError(err)
	return configMap, exists, err
}

// UpdateConfigMap updates the specified configMap.
func (w *ClientWrapper) UpdateConfigMap(configMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	return w.KubeClient.CoreV1().ConfigMaps(configMap.Namespace).Update(configMap)
}

// CreateConfigMap creates the specified configMap.
func (w *ClientWrapper) CreateConfigMap(configMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	return w.KubeClient.CoreV1().ConfigMaps(configMap.Namespace).Create(configMap)
}

// translateNotFoundError will translate a "not found" error to a boolean return
// value which indicates if the resource exists and a nil error.
func translateNotFoundError(err error) (bool, error) {
	if kubeerror.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// ParseServiceNamePort parses a name, namespace, and a port from a string, using the default namespace if none is defined.
func ParseServiceNamePort(value string) (name, namespace string, port int32, err error) {
	service := strings.Split(value, ":")
	if len(service) < 2 {
		return "", "", 0, fmt.Errorf("could not parse service into name and port")
	}
	port64, err := strconv.ParseInt(service[1], 10, 32)
	if err != nil {
		return "", "", 0, err
	}
	port = int32(port64)

	substring := strings.Split(service[0], "/")
	if len(substring) == 1 {
		return service[0], metav1.NamespaceDefault, port, nil
	}

	return substring[1], substring[0], port, nil
}

// ServiceNamePortToString formats a parseable string from the values.
func ServiceNamePortToString(name, namespace string, port int32) (value string) {
	return fmt.Sprintf("%s/%s:%d", namespace, name, port)
}
