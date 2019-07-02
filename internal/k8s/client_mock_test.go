package k8s

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/containous/traefik/pkg/provider/kubernetes/crd/traefik/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	//smiAccessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	//smiSpecsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	//smiSplitv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/split/v1alpha1"
	//smiAccessClientset "github.com/deislabs/smi-sdk-go/pkg/gen/client/access/clientset/versioned"
	//smiSpecsClientset "github.com/deislabs/smi-sdk-go/pkg/gen/client/specs/clientset/versioned"
	//smiSplitClientset "github.com/deislabs/smi-sdk-go/pkg/gen/client/split/clientset/versioned"
	//log "github.com/sirupsen/logrus"
	//appsv1 "k8s.io/api/apps/v1"
)

var _ CoreV1Client = (*coreV1ClientMock)(nil)

func init() {
	// required by k8s.MustParseYaml
	err := v1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}
}

type coreV1ClientMock struct {
	services     []*corev1.Service
	servicesList *corev1.ServiceList
	pods         []*corev1.Pod
	podsList     *corev1.PodList
	endpoints    []*corev1.Endpoints
	namespaces   []*corev1.Namespace
	configmaps   []*corev1.ConfigMap

	apiServiceError   error
	apiPodError       error
	apiEndpointsError error
	apiNamespaceError error
	apiConfigmapError error
}

func newCoreV1ClientMock(paths ...string) coreV1ClientMock {
	var c coreV1ClientMock

	for _, path := range paths {
		yamlContent, err := ioutil.ReadFile(filepath.FromSlash("./fixtures/" + path))
		if err != nil {
			panic(err)
		}

		k8sObjects := MustParseYaml(yamlContent)
		for _, obj := range k8sObjects {
			switch o := obj.(type) {
			case *corev1.Service:
				c.services = append(c.services, o)
			case *corev1.Pod:
				c.pods = append(c.pods, o)
			case *corev1.Endpoints:
				c.endpoints = append(c.endpoints, o)
			case *corev1.Namespace:
				c.namespaces = append(c.namespaces, o)
			default:
				panic(fmt.Sprintf("Unknown runtime object %+v %T", o, o))
			}
		}
	}

	return c
}

func (c coreV1ClientMock) GetService(namespace, name string) (*corev1.Service, bool, error) {
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

func (c coreV1ClientMock) GetServices(namespace string) ([]*corev1.Service, error) {
	if c.apiServiceError != nil {
		return nil, c.apiServiceError
	}

	return c.services, nil
}

func (c coreV1ClientMock) ListServicesWithOptions(namespace string, options metav1.ListOptions) (*corev1.ServiceList, error) {
	if c.apiServiceError != nil {
		return nil, c.apiServiceError
	}

	return c.servicesList, nil
}

func (c coreV1ClientMock) WatchServicesWithOptions(namespace string, options metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c coreV1ClientMock) DeleteService(namespace, name string) error {
	panic("implement me")
}

func (c coreV1ClientMock) CreateService(service *corev1.Service) (*corev1.Service, error) {
	panic("implement me")
}

func (c coreV1ClientMock) UpdateService(service *corev1.Service) (*corev1.Service, error) {
	panic("implement me")
}

func (c coreV1ClientMock) GetEndpoints(namespace, name string) (*corev1.Endpoints, bool, error) {
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

func (c coreV1ClientMock) GetPod(namespace, name string) (*corev1.Pod, bool, error) {
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

func (c coreV1ClientMock) ListPodWithOptions(namespace string, options metav1.ListOptions) (*corev1.PodList, error) {
	if c.apiPodError != nil {
		return nil, c.apiPodError
	}

	return c.podsList, nil
}

func (c coreV1ClientMock) GetNamespaces() ([]*corev1.Namespace, error) {
	return c.namespaces, c.apiNamespaceError
}

func (c coreV1ClientMock) GetConfigmap(namespace, name string) (*corev1.ConfigMap, bool, error) {
	if c.apiConfigmapError != nil {
		return nil, false, c.apiConfigmapError
	}

	for _, configmap := range c.configmaps {
		if configmap.Namespace == namespace && configmap.Name == name {
			return configmap, true, nil
		}
	}
	return nil, false, c.apiConfigmapError
}

func (c coreV1ClientMock) CreateConfigmap(service *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	panic("implement me")
}

func (c coreV1ClientMock) UpdateConfigmap(service *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	panic("implement me")
}
