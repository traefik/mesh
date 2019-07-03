package k8s

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	smiAccessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	smiSpecsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	smiSplitv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/split/v1alpha1"
	smiAccessClientset "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/clientset/versioned"
	smiSpecsClientset "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/clientset/versioned"
	smiSplitClientset "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/clientset/versioned"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kubeerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

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
	GetNamespaces() ([]*corev1.Namespace, error)
	GetConfigmap(namespace, name string) (*corev1.ConfigMap, bool, error)
	CreateConfigmap(configmap *corev1.ConfigMap) (*corev1.ConfigMap, error)
	UpdateConfigmap(configmap *corev1.ConfigMap) (*corev1.ConfigMap, error)
}

type AppsV1Client interface {
	GetDeployment(namespace, name string) (*appsv1.Deployment, bool, error)
}

type SMIAccessV1Alpha1Client interface {
	ListTrafficTargetsWithOptions(namespace string, options metav1.ListOptions) (*smiAccessv1alpha1.TrafficTargetList, error)
	WatchTrafficTargetsWithOptions(namespace string, options metav1.ListOptions) (watch.Interface, error)
	GetTrafficTargets() ([]*smiAccessv1alpha1.TrafficTarget, error)
}

type SMISpecsV1Alpha1Client interface {
	ListHTTPRouteGroupsWithOptions(namespace string, options metav1.ListOptions) (*smiSpecsv1alpha1.HTTPRouteGroupList, error)
	WatchHTTPRouteGroupsWithOptions(namespace string, options metav1.ListOptions) (watch.Interface, error)
	GetHTTPRouteGroup(namespace, name string) (*smiSpecsv1alpha1.HTTPRouteGroup, bool, error)
}

type SMISplitV1Alpha1Client interface {
	ListTrafficSplitsWithOptions(namespace string, options metav1.ListOptions) (*smiSplitv1alpha1.TrafficSplitList, error)
	WatchTrafficSplitsWithOptions(namespace string, options metav1.ListOptions) (watch.Interface, error)
}

// ClientWrapper holds the clients for the various resource controllers.
type ClientWrapper struct {
	KubeClient      *kubernetes.Clientset
	SmiAccessClient *smiAccessClientset.Clientset
	SmiSpecsClient  *smiSpecsClientset.Clientset
	SmiSplitClient  *smiSplitClientset.Clientset
}

// IgnoreWrapper holds namespaces and services to ignore.
type IgnoreWrapper struct {
	Namespaces Namespaces
	Services   Services
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

// InitCluster is used to initialize a kubernetes cluster with a variety of configuration options.
func (w *ClientWrapper) InitCluster() error {
	log.Infoln("Preparing Cluster...")

	log.Debugln("Creating mesh namespace...")
	if err := w.CreateNamespace(MeshNamespace); err != nil {
		return err
	}

	log.Debugln("Patching CoreDNS...")
	if err := w.patchCoreDNS("coredns", metav1.NamespaceSystem); err != nil {
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

	serverBlock :=
		`
traefik.mesh.svc.cluster.local:53 {
    errors
    rewrite continue {
        name regex ([a-zA-Z0-9-_]*)\.([a-zv0-9-_]*)\.traefik\.mesh traefik-{1}-{2}.traefik-mesh.svc.cluster.local
        answer name traefik-([a-zA-Z0-9-_]*)-([a-zA-Z0-9-_]*)\.traefik-mesh\.svc\.cluster\.local {1}.{2}.traefik.mesh
    }
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        upstream
    fallthrough in-addr.arpa ip6.arpa
    }
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}
`
	originalBlock := coreConfigMap.Data["Corefile"]
	newBlock := originalBlock + serverBlock
	coreConfigMap.Data["Corefile"] = newBlock
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

	// Never edit original object, always work with a clone for updates.
	newDeployment := coreDeployment.DeepCopy()
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
	if err := w.CreateNamespace(MeshNamespace); err != nil {
		return err
	}

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

// buildSmiAccessClient returns a client to manage SMI Access objects.
func buildSmiAccessClient(config *rest.Config) (*smiAccessClientset.Clientset, error) {
	log.Infoln("Building SMI Access Client...")
	client, err := smiAccessClientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Access Client: %v", err)
	}

	return client, nil
}

// buildSmiSpecsClient returns a client to manage SMI Specs objects.
func buildSmiSpecsClient(config *rest.Config) (*smiSpecsClientset.Clientset, error) {
	log.Infoln("Building SMI Specs Client...")
	client, err := smiSpecsClientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create SMI Specs Client: %v", err)
	}

	return client, nil
}

// buildSmiSplitClient returns a client to manage SMI Split objects.
func buildSmiSplitClient(config *rest.Config) (*smiSplitClientset.Clientset, error) {
	log.Infoln("Building SMI Split Client...")
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

// ListTrafficTargetsWithOptions lists trafficTargets with the specified options.
func (w *ClientWrapper) ListTrafficTargetsWithOptions(namespace string, options metav1.ListOptions) (*smiAccessv1alpha1.TrafficTargetList, error) {
	return w.SmiAccessClient.AccessV1alpha1().TrafficTargets(namespace).List(options)
}

// WatchTrafficTargetsWithOptions watches trafficTargets with the specified options.
func (w *ClientWrapper) WatchTrafficTargetsWithOptions(namespace string, options metav1.ListOptions) (watch.Interface, error) {
	return w.SmiAccessClient.AccessV1alpha1().TrafficTargets(namespace).Watch(options)
}

// GetTrafficTargets returns a slice of all TrafficTargets.
func (w *ClientWrapper) GetTrafficTargets() ([]*smiAccessv1alpha1.TrafficTarget, error) {
	var result []*smiAccessv1alpha1.TrafficTarget
	list, err := w.SmiAccessClient.AccessV1alpha1().TrafficTargets(metav1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return result, err
	}
	for _, trafficTarget := range list.Items {
		result = append(result, &trafficTarget)
	}
	return result, nil
}

// ListHTTPRouteGroupsWithOptions lists HTTPRouteGroups with the specified options.
func (w *ClientWrapper) ListHTTPRouteGroupsWithOptions(namespace string, options metav1.ListOptions) (*smiSpecsv1alpha1.HTTPRouteGroupList, error) {
	return w.SmiSpecsClient.SpecsV1alpha1().HTTPRouteGroups(namespace).List(options)
}

// WatchHTTPRouteGroupsWithOptions watches HTTPRouteGroups with the specified options.
func (w *ClientWrapper) WatchHTTPRouteGroupsWithOptions(namespace string, options metav1.ListOptions) (watch.Interface, error) {
	return w.SmiSpecsClient.SpecsV1alpha1().HTTPRouteGroups(namespace).Watch(options)
}

// GetHTTPRouteGroup retrieves the HTTPRouteGroup from the specified namespace.
func (w *ClientWrapper) GetHTTPRouteGroup(namespace, name string) (*smiSpecsv1alpha1.HTTPRouteGroup, bool, error) {
	group, err := w.SmiSpecsClient.SpecsV1alpha1().HTTPRouteGroups(namespace).Get(name, metav1.GetOptions{})
	exists, err := translateNotFoundError(err)
	return group, exists, err
}

// ListTrafficSplitsWithOptions lists TrafficSplits with the specified options.
func (w *ClientWrapper) ListTrafficSplitsWithOptions(namespace string, options metav1.ListOptions) (*smiSplitv1alpha1.TrafficSplitList, error) {
	return w.SmiSplitClient.SplitV1alpha1().TrafficSplits(namespace).List(options)
}

// WatchTrafficTargetsWithOptions watches trafficTargets with the specified options.
func (w *ClientWrapper) WatchTrafficSplitsWithOptions(namespace string, options metav1.ListOptions) (watch.Interface, error) {
	return w.SmiSplitClient.SplitV1alpha1().TrafficSplits(namespace).Watch(options)
}

// GetConfigmap retrieves the named configmap in the specified namespace.
func (w *ClientWrapper) GetConfigmap(namespace, name string) (*corev1.ConfigMap, bool, error) {
	configmap, err := w.KubeClient.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
	exists, err := translateNotFoundError(err)
	return configmap, exists, err
}

// UpdateConfigmap updates the specified service.
func (w *ClientWrapper) UpdateConfigmap(configmap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	return w.KubeClient.CoreV1().ConfigMaps(configmap.Namespace).Update(configmap)
}

// CreateConfigmap creates the specified service.
func (w *ClientWrapper) CreateConfigmap(configmap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	return w.KubeClient.CoreV1().ConfigMaps(configmap.Namespace).Create(configmap)
}

// translateNotFoundError will translate a "not found" error to a boolean return
// value which indicates if the resource exists and a nil error.
func translateNotFoundError(err error) (bool, error) {
	if kubeerror.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// MustParseYaml parses a YAML to objects.
func MustParseYaml(content []byte) []runtime.Object {
	acceptedK8sTypes := regexp.MustCompile(`(Deployment|Endpoints|Service|Ingress|IngressRoute|Middleware|Secret|TLSOption)`)

	files := strings.Split(string(content), "---")
	retVal := make([]runtime.Object, 0, len(files))
	for _, file := range files {
		if file == "\n" || file == "" {
			continue
		}

		decode := scheme.Codecs.UniversalDeserializer().Decode
		obj, groupVersionKind, err := decode([]byte(file), nil, nil)
		if err != nil {
			panic(fmt.Sprintf("Error while decoding YAML object. Err was: %s", err))
		}

		if !acceptedK8sTypes.MatchString(groupVersionKind.Kind) {
			log.Debugf("The custom-roles configMap contained K8s object types which are not supported! Skipping object with type: %s", groupVersionKind.Kind)
		} else {
			retVal = append(retVal, obj)
		}
	}
	return retVal
}
