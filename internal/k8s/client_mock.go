package k8s

import (
	"errors"
	"fmt"
	"io/ioutil"

	"path/filepath"
	"regexp"
	"strings"

	"github.com/containous/traefik/v2/pkg/provider/kubernetes/crd/traefik/v1alpha1"
	accessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	specsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	splitv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/split/v1alpha1"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ CoreV1Client = (*CoreV1ClientMock)(nil)

func init() {
	// required by k8s.MustParseYaml
	err := accessv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}

	err = specsv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}

	err = splitv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}

	err = v1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}
}

// CoreV1ClientMock holds CoreV1 client mock information.
type CoreV1ClientMock struct {
	services     []*corev1.Service
	servicesList *corev1.ServiceList
	pods         []*corev1.Pod
	endpoints    []*corev1.Endpoints
	namespaces   []*corev1.Namespace
	configMaps   []*corev1.ConfigMap

	apiServiceError   error
	apiPodError       error
	apiEndpointsError error
	apiNamespaceError error
	apiConfigMapError error
}

// AppsV1ClientMock holds AppsV1 client mock information.
type AppsV1ClientMock struct {
	deployments []*appsv1.Deployment

	apiDeploymentError error
}

// SMIClientMock holds SMI client mock information.
type SMIClientMock struct {
	trafficTargets  []*accessv1alpha1.TrafficTarget
	httpRouteGroups []*specsv1alpha1.HTTPRouteGroup
	tcpRoutes 		[]*specsv1alpha1.TCPRoute
	trafficSplits   []*splitv1alpha1.TrafficSplit

	apiTrafficTargetError  error
	apiHTTPRouteGroupError error
	apiTCPRouteError error
	apiTrafficSplitError   error
}

// ClientMock clients mock.
type ClientMock struct {
	CoreV1ClientMock
	SMIClientMock
	AppsV1ClientMock
}

// NewCoreV1ClientMock create a new corev1 client mock.
func NewCoreV1ClientMock(paths ...string) *CoreV1ClientMock {
	c := &CoreV1ClientMock{}

	for _, path := range paths {
		yamlContent, err := ioutil.ReadFile(filepath.FromSlash("./fixtures/" + path))
		if err != nil {
			panic(err)
		}

		k8sObjects := MustParseYaml(yamlContent)
		for _, obj := range k8sObjects {
			switch o := obj.(type) {
			case *corev1.Service:
				setNamespaceIfNot(o)
				c.services = append(c.services, o)
			case *corev1.Pod:
				setNamespaceIfNot(o)
				c.pods = append(c.pods, o)
			case *corev1.Endpoints:
				setNamespaceIfNot(o)
				c.endpoints = append(c.endpoints, o)
			case *corev1.Namespace:
				setNamespaceIfNot(o)
				c.namespaces = append(c.namespaces, o)
			default:
				panic(fmt.Sprintf("Unknown runtime object %+v %T", o, o))
			}
		}
	}

	return c
}

// NewSMIClientMock create a new smi client mock.
func NewSMIClientMock(paths ...string) *SMIClientMock {
	s := &SMIClientMock{}

	for _, path := range paths {
		yamlContent, err := ioutil.ReadFile(filepath.FromSlash("./fixtures/" + path))
		if err != nil {
			panic(err)
		}

		k8sObjects := MustParseYaml(yamlContent)
		for _, obj := range k8sObjects {
			switch o := obj.(type) {
			case *accessv1alpha1.TrafficTarget:
				setNamespaceIfNot(o)
				s.trafficTargets = append(s.trafficTargets, o)
			case *specsv1alpha1.HTTPRouteGroup:
				setNamespaceIfNot(o)
				s.httpRouteGroups = append(s.httpRouteGroups, o)
			case *splitv1alpha1.TrafficSplit:
				setNamespaceIfNot(o)
				s.trafficSplits = append(s.trafficSplits, o)
			default:
				panic(fmt.Sprintf("Unknown runtime object %+v %T", o, o))
			}
		}
	}

	return s
}

// NewClientMock create a new client mock.
func NewClientMock(paths ...string) *ClientMock {
	c := &ClientMock{}

	for _, path := range paths {
		yamlContent, err := ioutil.ReadFile(filepath.FromSlash("./fixtures/" + path))
		if err != nil {
			panic(err)
		}

		k8sObjects := MustParseYaml(yamlContent)
		for _, obj := range k8sObjects {
			switch o := obj.(type) {
			case *corev1.Service:
				setNamespaceIfNot(o)
				c.services = append(c.services, o)
			case *corev1.Pod:
				setNamespaceIfNot(o)
				c.pods = append(c.pods, o)
			case *corev1.Endpoints:
				setNamespaceIfNot(o)
				c.endpoints = append(c.endpoints, o)
			case *corev1.Namespace:
				setNamespaceIfNot(o)
				c.namespaces = append(c.namespaces, o)
			case *accessv1alpha1.TrafficTarget:
				setNamespaceIfNot(o)
				c.trafficTargets = append(c.trafficTargets, o)
			case *specsv1alpha1.HTTPRouteGroup:
				setNamespaceIfNot(o)
				c.httpRouteGroups = append(c.httpRouteGroups, o)
			case *splitv1alpha1.TrafficSplit:
				setNamespaceIfNot(o)
				c.trafficSplits = append(c.trafficSplits, o)
			default:
				panic(fmt.Sprintf("Unknown runtime object %+v %T", o, o))
			}
		}
	}

	return c
}

func setNamespaceIfNot(obj metav1.Object) {
	if obj.GetNamespace() == "" {
		obj.SetNamespace(metav1.NamespaceDefault)
	}
}

// GetService returns mocked date for service.
func (c *CoreV1ClientMock) GetService(namespace, name string) (*corev1.Service, bool, error) {
	if c.apiServiceError != nil {
		return nil, false, c.apiServiceError
	}

	for _, service := range c.services {
		if service.Namespace == namespace && service.Name == name {
			return service, true, nil
		}
	}

	return nil, false, c.apiServiceError
}

// GetServices returns mocked date for services.
func (c *CoreV1ClientMock) GetServices(namespace string) ([]*corev1.Service, error) {
	if c.apiServiceError != nil {
		return nil, c.apiServiceError
	}

	return c.services, nil
}

// ListServicesWithOptions returns mocked date for services.
func (c *CoreV1ClientMock) ListServicesWithOptions(namespace string, options metav1.ListOptions) (*corev1.ServiceList, error) {
	if c.apiServiceError != nil {
		return nil, c.apiServiceError
	}

	return c.servicesList, nil
}

// WatchServicesWithOptions mocks service watch.
func (c *CoreV1ClientMock) WatchServicesWithOptions(namespace string, options metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

// DeleteService mocks service delete.
func (c *CoreV1ClientMock) DeleteService(namespace, name string) error {
	panic("implement me")
}

// CreateService mocks service update.
func (c *CoreV1ClientMock) CreateService(service *corev1.Service) (*corev1.Service, error) {
	panic("implement me")
}

// UpdateService mocks service update.
func (c *CoreV1ClientMock) UpdateService(service *corev1.Service) (*corev1.Service, error) {
	panic("implement me")
}

// GetEndpoints returns mocked data for endpoints.
func (c *CoreV1ClientMock) GetEndpoints(namespace, name string) (*corev1.Endpoints, bool, error) {
	if c.apiEndpointsError != nil {
		return nil, false, c.apiEndpointsError
	}

	for _, endpoint := range c.endpoints {
		if endpoint.Namespace == namespace && endpoint.Name == name {
			return endpoint, true, nil
		}
	}

	return nil, false, c.apiEndpointsError
}

// GetEndpointses returns mocked date for endpoints.
func (c *CoreV1ClientMock) GetEndpointses(namespace string) ([]*corev1.Endpoints, error) {
	if c.apiEndpointsError != nil {
		return nil, c.apiEndpointsError
	}

	return c.endpoints, nil
}

// GetPod returns mocked data for pod.
func (c *CoreV1ClientMock) GetPod(namespace, name string) (*corev1.Pod, bool, error) {
	if c.apiPodError != nil {
		return nil, false, c.apiPodError
	}

	for _, pod := range c.pods {
		if pod.Namespace == namespace && pod.Name == name {
			return pod, true, nil
		}
	}

	return nil, false, c.apiPodError
}

// ListPodWithOptions returns mocked data for pods.
func (c *CoreV1ClientMock) ListPodWithOptions(namespace string, options metav1.ListOptions) (*corev1.PodList, error) {
	if c.apiPodError != nil {
		return nil, c.apiPodError
	}

	var items []corev1.Pod

	for _, pod := range c.pods {
		items = append(items, *pod)
	}

	result := &corev1.PodList{
		Items: items,
	}

	return result, nil
}

// GetNamespace returns mocked data for namespace.
func (c *CoreV1ClientMock) GetNamespace(name string) (*corev1.Namespace, bool, error) {
	if c.apiNamespaceError != nil {
		return nil, false, c.apiNamespaceError
	}

	for _, ns := range c.namespaces {
		if ns.Name == name {
			return ns, true, nil
		}
	}

	return nil, false, c.apiNamespaceError
}

// GetNamespaces returns mocked data for namespaces.
func (c *CoreV1ClientMock) GetNamespaces() ([]*corev1.Namespace, error) {
	if c.apiNamespaceError != nil {
		return nil, c.apiNamespaceError
	}

	return c.namespaces, nil
}

// GetConfigMap returns mocked data for config map.
func (c *CoreV1ClientMock) GetConfigMap(namespace, name string) (*corev1.ConfigMap, bool, error) {
	if c.apiConfigMapError != nil {
		return nil, false, c.apiConfigMapError
	}

	for _, configmap := range c.configMaps {
		if configmap.Namespace == namespace && configmap.Name == name {
			return configmap, true, nil
		}
	}

	return nil, false, c.apiConfigMapError
}

// CreateConfigMap mock config map create.
func (c *CoreV1ClientMock) CreateConfigMap(configmap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	panic("implement me")
}

// UpdateConfigMap mock config map update.
func (c *CoreV1ClientMock) UpdateConfigMap(configmap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	panic("implement me")
}

// EnableEndpointsError enables error on endpoints.
func (c *CoreV1ClientMock) EnableEndpointsError() {
	c.apiEndpointsError = errors.New("endpoint error")
}

// EnableNamespaceError enables error on namespace.
func (c *CoreV1ClientMock) EnableNamespaceError() {
	c.apiNamespaceError = errors.New("namespace error")
}

// EnableServiceError enables error on service.
func (c *CoreV1ClientMock) EnableServiceError() {
	c.apiServiceError = errors.New("service error")
}

// EnablePodError enables error on pod.
func (c *CoreV1ClientMock) EnablePodError() {
	c.apiPodError = errors.New("pod error")
}

// GetDeployment returns mocked data for deployment.
func (a *AppsV1ClientMock) GetDeployment(namespace, name string) (*appsv1.Deployment, bool, error) {
	if a.apiDeploymentError != nil {
		return nil, false, a.apiDeploymentError
	}

	for _, deployment := range a.deployments {
		if deployment.Name == name && deployment.Namespace == namespace {
			return deployment, true, nil
		}
	}

	return nil, false, a.apiDeploymentError
}

// UpdateDeployment mocked deployment update.
func (a *AppsV1ClientMock) UpdateDeployment(deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	panic("implement me")
}

// GetHTTPRouteGroup returns mocked data for HTTP route group.
func (s *SMIClientMock) GetHTTPRouteGroup(namespace, name string) (*specsv1alpha1.HTTPRouteGroup, bool, error) {
	if s.apiHTTPRouteGroupError != nil {
		return nil, false, s.apiHTTPRouteGroupError
	}

	for _, hrg := range s.httpRouteGroups {
		if hrg.Name == name && hrg.Namespace == namespace {
			return hrg, true, nil
		}
	}

	return nil, false, s.apiHTTPRouteGroupError
}

// GetHTTPRouteGroup returns mocked data for HTTP route group.
func (s *SMIClientMock) GetTCPRoute(namespace, name string) (*specsv1alpha1.TCPRoute, bool, error) {
	if s.apiTCPRouteError != nil {
		return nil, false, s.apiTCPRouteError
	}

	for _, hrg := range s.tcpRoutes {
		if hrg.Name == name && hrg.Namespace == namespace {
			return hrg, true, nil
		}
	}

	return nil, false, s.apiTCPRouteError
}

// GetTrafficTargets returns mocked data for traffic targets.
func (s *SMIClientMock) GetTrafficTargets() ([]*accessv1alpha1.TrafficTarget, error) {
	if s.apiTrafficTargetError != nil {
		return nil, s.apiTrafficTargetError
	}

	return s.trafficTargets, nil
}

// GetTrafficSplits returns mocked data for traffic splits.
func (s *SMIClientMock) GetTrafficSplits() ([]*splitv1alpha1.TrafficSplit, error) {
	if s.apiTrafficSplitError != nil {
		return nil, s.apiTrafficSplitError
	}

	return s.trafficSplits, nil
}

// EnableTrafficTargetError enables error on traffic target.
func (s *SMIClientMock) EnableTrafficTargetError() {
	s.apiTrafficTargetError = errors.New("trafficTarget error")
}

// EnableHTTPRouteGroupError enables error on http router group.
func (s *SMIClientMock) EnableHTTPRouteGroupError() {
	s.apiHTTPRouteGroupError = errors.New("httpRouteGroup error")
}

// EnableTrafficSplitError enables error on traffic split.
func (s *SMIClientMock) EnableTrafficSplitError() {
	s.apiTrafficSplitError = errors.New("trafficSplit error")
}

// MustParseYaml parses a YAML to objects.
func MustParseYaml(content []byte) []runtime.Object {
	acceptedK8sTypes := regexp.MustCompile(`(Deployment|Endpoints|Service|Ingress|Middleware|Secret|TLSOption|Namespace|TrafficTarget|HTTPRouteGroup|TrafficSplit|Pod)`)

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
