package kubernetes

import (
	"testing"

	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/traefik/pkg/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildRouterFromService(t *testing.T) {
	testCases := []struct {
		desc     string
		service  *corev1.Service
		expected *config.Router
	}{
		{
			desc: "",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "foo",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.0.0.1",
				},
			},
			expected: &config.Router{
				Rule: "Host(`test.foo.traefik.mesh`) || Host(`10.0.0.1`)",
			},
		},
	}

	provider := New(nil)

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			actual := provider.buildRouter(test.service.Name, test.service.Namespace, test.service.Spec.ClusterIP, 50, "")
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestBuildConfiguration(t *testing.T) {
	testCases := []struct {
		desc         string
		mockFile     string
		expected     *config.Configuration
		namespaceErr bool
		serviceErr   bool
	}{
		{
			desc:     "simple configuration build",
			mockFile: "build_configuration_simple.yaml",
			expected: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers: map[string]*config.Router{
						"00757dccfb93dcceecc29c6ed96bea635ec513f0665591c171b24ca767009643": {
							Rule: "Host(`test.foo.traefik.mesh`) || Host(`10.1.0.1`)",
						},
					},
					Services: map[string]*config.Service{
						"00757dccfb93dcceecc29c6ed96bea635ec513f0665591c171b24ca767009643": {
							LoadBalancer: &config.LoadBalancerService{
								PassHostHeader: true,
								Servers: []config.Server{
									{
										URL: "http://10.0.0.1:80",
									},
									{
										URL: "http://10.0.0.2:80",
									},
								},
							},
						}},
				},
			},
		},
		{
			desc:     "namespace error",
			mockFile: "build_configuration_simple.yaml",
			expected: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
			},
			namespaceErr: true,
		},
		{
			desc:     "service error",
			mockFile: "build_configuration_simple.yaml",
			expected: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
			},
			serviceErr: true,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			clientMock := k8s.NewCoreV1ClientMock(test.mockFile)
			if test.namespaceErr {
				clientMock.EnableNamespaceError()
			}
			if test.serviceErr {
				clientMock.EnableServiceError()
			}

			//provider := New(clientMock)
			actual := 10 //provider.BuildConfiguration()
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestBuildServiceFromService(t *testing.T) {
	testCases := []struct {
		desc     string
		mockFile string
		expected *config.Service
		err      bool
	}{
		{
			desc:     "two successful endpoints",
			mockFile: "build_service_from_service_simple.yaml",
			expected: &config.Service{
				LoadBalancer: &config.LoadBalancerService{
					PassHostHeader: true,
					Servers: []config.Server{
						{
							URL: "http://10.0.0.1:80",
						},
						{
							URL: "http://10.0.0.2:80",
						},
					},
				},
			},
		},
		{
			desc:     "missing endpoints found",
			mockFile: "build_service_from_service_missing_endpoints.yaml",
			expected: nil,
		},
		{
			desc:     "endpoints client error",
			mockFile: "build_service_from_service_simple.yaml",
			expected: nil,
			err:      true,
		},
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "foo",
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			clientMock := k8s.NewCoreV1ClientMock(test.mockFile)
			if test.err {
				clientMock.EnableEndpointsError()
			}
			provider := New(clientMock)
			actual := provider.buildService(service.Name, service.Namespace)
			assert.Equal(t, test.expected, actual)

		})
	}
}
