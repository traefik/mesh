package k8s

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v3"
	"github.com/containous/traefik/v2/pkg/safe"

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
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	supportedCoreDNSVersions = []string{
		"1.3",
		"1.4",
		"1.5",
		"1.6",
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

// CoreV1Client CoreV1 client.
type CoreV1Client interface {
	GetNamespace(name string) (*corev1.Namespace, bool, error)
	GetNamespaces() ([]*corev1.Namespace, error)

	GetService(namespace, name string) (*corev1.Service, bool, error)
	GetServices(namespace string) ([]*corev1.Service, error)
	ListServicesWithOptions(namespace string, options metav1.ListOptions) (*corev1.ServiceList, error)
	WatchServicesWithOptions(namespace string, options metav1.ListOptions) (watch.Interface, error)
	CreateService(service *corev1.Service) (*corev1.Service, error)
	UpdateService(service *corev1.Service) (*corev1.Service, error)
	DeleteService(namespace, name string) error

	GetEndpoints(namespace, name string) (*corev1.Endpoints, bool, error)
	GetEndpointses(namespace string) ([]*corev1.Endpoints, error)

	GetPod(namespace, name string) (*corev1.Pod, bool, error)
	ListPodWithOptions(namespace string, options metav1.ListOptions) (*corev1.PodList, error)

	GetConfigMap(namespace, name string) (*corev1.ConfigMap, bool, error)
	UpdateConfigMap(configMap *corev1.ConfigMap) (*corev1.ConfigMap, error)
	CreateConfigMap(configMap *corev1.ConfigMap) (*corev1.ConfigMap, error)
}

// AppsV1Client AppsV1 client.
type AppsV1Client interface {
	GetDeployment(namespace, name string) (*appsv1.Deployment, bool, error)
	UpdateDeployment(deployment *appsv1.Deployment) (*appsv1.Deployment, error)
}

// SMIClient SMI client.
type SMIClient interface {
	SMIAccessV1Alpha1Client
	SMISpecsV1Alpha1Client
	SMISplitV1Alpha1Client
}

// SMIAccessV1Alpha1Client SMI Access v1Alpha client.
type SMIAccessV1Alpha1Client interface {
	GetTrafficTargets() ([]*smiAccessv1alpha1.TrafficTarget, error)
}

// SMISpecsV1Alpha1Client SMI Specs v1Alpha client.
type SMISpecsV1Alpha1Client interface {
	GetHTTPRouteGroup(namespace, name string) (*smiSpecsv1alpha1.HTTPRouteGroup, bool, error)
}

// SMISplitV1Alpha1Client SMI Split v1Alpha client.
type SMISplitV1Alpha1Client interface {
	GetTrafficSplits() ([]*smiSplitv1alpha1.TrafficSplit, error)
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

	match, err := w.CoreDNSMatch()
	if err != nil {
		return err
	}

	if !match {
		match, err = w.KubeDNSMatch()
		if err != nil {
			return err
		}
	}

	if !match {
		return fmt.Errorf("no core dns service available for installing maesh: %v", err)
	}

	return nil

}

// CoreDNSMatch checks if CoreDNS service can match.
func (w *ClientWrapper) CoreDNSMatch() (bool, error) {
	log.Infoln("Checking CoreDNS...")
	log.Debugln("Get CoreDNS version...")
	deployment, exists, err := w.GetDeployment(metav1.NamespaceSystem, "coredns")
	if err != nil {
		return false, fmt.Errorf("unable to get deployment %q in namesapce %q: %v", "coredns", metav1.NamespaceSystem, err)
	}

	if !exists {
		log.Debugf("%s does not exist in namespace %s", "coredns", metav1.NamespaceSystem)
		return false, nil
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
		return false, fmt.Errorf("unsupported CoreDNS version %q, (supported versions are: %s)", version, strings.Join(supportedCoreDNSVersions, ","))
	}

	log.Info("CoreDNS match")
	return true, nil
}

// KubeDNSMatch checks if KubeDNS service can match.
func (w *ClientWrapper) KubeDNSMatch() (bool, error) {
	log.Infoln("Checking KubeDNS...")
	log.Debugln("Get KubeDNS version...")
	_, exists, err := w.GetDeployment(metav1.NamespaceSystem, "kube-dns")
	if err != nil {
		return false, fmt.Errorf("unable to get deployment %q in namesapce %q: %v", "kube-dns", metav1.NamespaceSystem, err)
	}

	if !exists {
		log.Debugf("%s does not exist in namespace %s", "kube-dns", metav1.NamespaceSystem)
		return false, nil
	}

	log.Info("KubeDNS match")
	return true, nil
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

	log.Debugln("Patching DNS...")
	if err := w.patchDNS(metav1.NamespaceSystem); err != nil {
		return err
	}

	log.Infoln("Cluster Preparation Complete...")

	return nil
}

func (w *ClientWrapper) patchDNS(namespace string) error {
	deployment, exist, err := w.GetDeployment(namespace, "coredns")
	if err != nil {
		return err
	}

	// If CoreDNS exist we will patch it.
	if exist {
		log.Debugln("Patching CoreDNS configmap...")
		var patched bool
		patched, err = w.patchCoreDNSConfigMap(deployment)
		if err != nil {
			return err
		}

		if !patched {
			log.Debugln("Restarting CoreDNS pods...")
			if err = w.restartPods(deployment); err != nil {
				return err
			}

			return nil
		}

		return nil
	}

	log.Debugln("coredns not available fallback to kube-dns")
	// If coreDNS does not exist we try to get the kube-dns
	deployment, exist, err = w.GetDeployment(namespace, "kube-dns")
	if err != nil {
		return err
	}

	if !exist {
		return fmt.Errorf("nor CoreDNS and KubeDNS are available in namespace %q", namespace)
	}

	ebo := backoff.NewConstantBackOff(10 * time.Second)

	var serviceIP string
	log.Debugln("Get CoreDNS service IP")
	if err = backoff.Retry(safe.OperationWithRecover(func() error {
		svc, exists, errSvc := w.GetService("maesh", "coredns")
		if errSvc != nil {
			return fmt.Errorf("unable get the service %q in namespace %q: %v", "coredns", "maesh", errSvc)
		}
		if !exists {
			return fmt.Errorf("service %q has not been yet created", "coredns")
		}
		if svc.Spec.ClusterIP == "" {
			return fmt.Errorf("service %q has no clusterIP", "coredns")
		}

		serviceIP = svc.Spec.ClusterIP
		return nil
	}), ebo); err != nil {
		return fmt.Errorf("unable get the service %q in namespace %q: %v", "coredns", "maesh", err)
	}

	// Patch KubeDNS
	log.Debugln("Patching KubeDNS configmap... with IP: ", serviceIP)
	patched, err := w.patchKubeDNSConfigMap(deployment, serviceIP)
	if err != nil {
		return err
	}

	if !patched {
		log.Debugln("Restarting KubeDNS pods...")
		if err := w.restartPods(deployment); err != nil {
			return err
		}
	}

	return nil
}

func (w *ClientWrapper) patchCoreDNSConfigMap(coreDeployment *appsv1.Deployment) (bool, error) {
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
    errors
    rewrite continue {
        name regex ([a-zA-Z0-9-_]*)\.([a-zv0-9-_]*)\.maesh maesh-{1}-{2}.maesh.svc.cluster.local
        answer name maesh-([a-zA-Z0-9-_]*)-([a-zA-Z0-9-_]*)\.maesh\.svc\.cluster\.local {1}.{2}.maesh
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
	coreConfigMap.ObjectMeta.Labels["maesh-patched"] = "true"

	if _, err = w.KubeClient.CoreV1().ConfigMaps(coreDeployment.Namespace).Update(coreConfigMap); err != nil {
		return false, err
	}

	return false, nil
}

func (w *ClientWrapper) patchKubeDNSConfigMap(deployment *appsv1.Deployment, coreDNSIp string) (bool, error) {
	var configMapName string
	if len(deployment.Spec.Template.Spec.Volumes) == 0 {
		return false, errors.New("kube-dns configmap not defined")
	}

	configMapName = deployment.Spec.Template.Spec.Volumes[0].ConfigMap.Name

	configMap, err := w.KubeClient.CoreV1().ConfigMaps(deployment.Namespace).Get(configMapName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	if len(configMap.ObjectMeta.Labels) > 0 {
		if _, ok := configMap.ObjectMeta.Labels["maesh-patched"]; ok {
			log.Debugln("Configmap already patched...")
			return true, nil
		}
	}

	stubDomains := make(map[string][]string)
	originalBlock, exist := configMap.Data["stubDomains"]
	if !exist {
		originalBlock = "{}"
	}

	if err = json.Unmarshal([]byte(originalBlock), &stubDomains); err != nil {
		return false, err
	}

	stubDomains["maesh"] = []string{coreDNSIp}
	var newData []byte
	newData, err = json.Marshal(stubDomains)
	if err != nil {
		return false, err
	}

	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}
	configMap.Data["stubDomains"] = string(newData)

	if len(configMap.ObjectMeta.Labels) == 0 {
		configMap.ObjectMeta.Labels = make(map[string]string)
	}
	configMap.ObjectMeta.Labels["maesh-patched"] = "true"

	if _, err = w.KubeClient.CoreV1().ConfigMaps(deployment.Namespace).Update(configMap); err != nil {
		return false, err
	}

	return false, nil
}

func (w *ClientWrapper) restartPods(deployment *appsv1.Deployment) error {
	log.Infof("Restarting %s pods...\n", deployment.Name)

	// Never edit original object, always work with a clone for updates.
	newDeployment := deployment.DeepCopy()
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

// GetEndpointses retrieves the endpoints from all namespaces.
func (w *ClientWrapper) GetEndpointses(namespace string) ([]*corev1.Endpoints, error) {
	var result []*corev1.Endpoints
	list, err := w.KubeClient.CoreV1().Endpoints(namespace).List(metav1.ListOptions{})
	if err != nil {
		return result, err
	}
	for _, endpoints := range list.Items {
		result = append(result, &endpoints)
	}
	return result, nil
}

// GetPod retrieves the pod from the specified namespace.
func (w *ClientWrapper) GetPod(namespace, name string) (*corev1.Pod, bool, error) {
	pod, err := w.KubeClient.CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
	exists, err := translateNotFoundError(err)
	return pod, exists, err
}

// ListPodWithOptions retrieves pods from the specified namespace.
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
		result = append(result, trafficTarget.DeepCopy())
	}
	return result, nil
}

// GetTrafficSplits returns a slice of all TrafficSplit.
func (w *ClientWrapper) GetTrafficSplits() ([]*smiSplitv1alpha1.TrafficSplit, error) {
	var result []*smiSplitv1alpha1.TrafficSplit
	list, err := w.SmiSplitClient.SplitV1alpha1().TrafficSplits(metav1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return result, err
	}
	for _, trafficSplit := range list.Items {
		result = append(result, trafficSplit.DeepCopy())
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

// ServiceNamePortToString formats a parsable string from the values.
func ServiceNamePortToString(name, namespace string, port int32) (value string) {
	return fmt.Sprintf("%s/%s:%d", namespace, name, port)
}
