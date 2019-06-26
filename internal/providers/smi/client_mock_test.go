package smi

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/containous/traefik/pkg/provider/kubernetes/crd/traefik/v1alpha1"
	"github.com/containous/traefik/pkg/provider/kubernetes/k8s"
	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	v1beta12 "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ Client = (*clientMock)(nil)

func init() {
	// required by k8s.MustParseYaml
	err := v1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}
}

type clientMock struct {
	ingresses []*extensionsv1beta1.Ingress
	services  []*corev1.Service
	pods      []*corev1.Pod
	secrets   []*corev1.Secret
	endpoints []*corev1.Endpoints

	apiServiceError       error
	apiSecretError        error
	apiEndpointsError     error
	apiIngressStatusError error

	ingressRoutes    []*v1alpha1.IngressRoute
	ingressRouteTCPs []*v1alpha1.IngressRouteTCP
	middlewares      []*v1alpha1.Middleware
	tlsOptions       []*v1alpha1.TLSOption

	watchChan chan interface{}
}

func newClientMock(paths ...string) clientMock {
	var c clientMock

	for _, path := range paths {
		yamlContent, err := ioutil.ReadFile(filepath.FromSlash("./fixtures/" + path))
		if err != nil {
			panic(err)
		}

		k8sObjects := k8s.MustParseYaml(yamlContent)
		for _, obj := range k8sObjects {
			switch o := obj.(type) {
			case *corev1.Service:
				c.services = append(c.services, o)
			case *corev1.Pod:
				c.pods = append(c.pods, o)
			case *corev1.Endpoints:
				c.endpoints = append(c.endpoints, o)
			case *v1alpha1.IngressRoute:
				c.ingressRoutes = append(c.ingressRoutes, o)
			case *v1alpha1.IngressRouteTCP:
				c.ingressRouteTCPs = append(c.ingressRouteTCPs, o)
			case *v1alpha1.Middleware:
				c.middlewares = append(c.middlewares, o)
			case *v1alpha1.TLSOption:
				c.tlsOptions = append(c.tlsOptions, o)
			case *v1beta12.Ingress:
				c.ingresses = append(c.ingresses, o)
			case *corev1.Secret:
				c.secrets = append(c.secrets, o)
			default:
				panic(fmt.Sprintf("Unknown runtime object %+v %T", o, o))
			}
		}
	}

	return c
}

func (c clientMock) GetIngressRoutes() []*v1alpha1.IngressRoute {
	return c.ingressRoutes
}

func (c clientMock) GetIngressRouteTCPs() []*v1alpha1.IngressRouteTCP {
	return c.ingressRouteTCPs
}

func (c clientMock) GetMiddlewares() []*v1alpha1.Middleware {
	return c.middlewares
}

func (c clientMock) GetTLSOptions() []*v1alpha1.TLSOption {
	return c.tlsOptions
}

func (c clientMock) GetTLSOption(namespace, name string) (*v1alpha1.TLSOption, bool, error) {
	for _, option := range c.tlsOptions {
		if option.Namespace == namespace && option.Name == name {
			return option, true, nil
		}
	}

	return nil, false, nil
}

func (c clientMock) GetIngresses() []*extensionsv1beta1.Ingress {
	return c.ingresses
}

func (c clientMock) GetService(namespace, name string) (*corev1.Service, bool, error) {
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

func (c clientMock) GetPod(namespace, name string) (*corev1.Pod, bool, error) {
	if c.apiServiceError != nil {
		return nil, false, c.apiServiceError
	}

	for _, pod := range c.pods {
		if pod.Namespace == namespace && pod.Name == name {
			return pod, true, nil
		}
	}
	return nil, false, c.apiServiceError
}

func (c clientMock) GetEndpoints(namespace, name string) (*corev1.Endpoints, bool, error) {
	if c.apiEndpointsError != nil {
		return nil, false, c.apiEndpointsError
	}

	for _, endpoints := range c.endpoints {
		if endpoints.Namespace == namespace && endpoints.Name == name {
			return endpoints, true, nil
		}
	}

	return &corev1.Endpoints{}, false, nil
}

func (c clientMock) GetSecret(namespace, name string) (*corev1.Secret, bool, error) {
	if c.apiSecretError != nil {
		return nil, false, c.apiSecretError
	}

	for _, secret := range c.secrets {
		if secret.Namespace == namespace && secret.Name == name {
			return secret, true, nil
		}
	}
	return nil, false, nil
}

func (c clientMock) WatchAll(namespaces []string, stopCh <-chan struct{}) (<-chan interface{}, error) {
	return c.watchChan, nil
}

func (c clientMock) UpdateIngressStatus(namespace, name, ip, hostname string) error {
	return c.apiIngressStatusError
}
