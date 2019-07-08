package kubernetes

import (
	"testing"

	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/i3o/internal/message"
	"github.com/containous/traefik/pkg/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildRouter(t *testing.T) {

	expected := &config.Router{
		Rule:        "Host(`test.foo.traefik.mesh`) || Host(`10.0.0.1`)",
		EntryPoints: []string{"ingress-80"},
		Service:     "bar",
	}

	provider := New(nil)

	name := "test"
	namespace := "foo"
	ip := "10.0.0.1"
	port := 80
	associatedService := "bar"

	actual := provider.buildRouter(name, namespace, ip, port, associatedService)
	assert.Equal(t, expected, actual)
}

func TestBuildConfiguration(t *testing.T) {
	testCases := []struct {
		desc           string
		mockFile       string
		event          message.Message
		provided       *config.Configuration
		expected       *config.Configuration
		endpointsError bool
	}{
		{
			desc:     "simple configuration build with empty event",
			mockFile: "build_configuration_simple.yaml",
			expected: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
			},
			provided: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
			},
		},
		{
			desc:     "simple configuration build with service event",
			mockFile: "build_configuration_simple.yaml",
			expected: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers: map[string]*config.Router{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							EntryPoints: []string{"ingress-5000"},
							Service:     "6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839",
							Rule:        "Host(`test.foo.traefik.mesh`) || Host(`10.1.0.1`)",
						},
					},
					Services: map[string]*config.Service{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							LoadBalancer: &config.LoadBalancerService{
								PassHostHeader: true,
								Servers: []config.Server{
									{
										URL:    "http://10.0.0.1:80",
										Scheme: "",
										Port:   "",
									},
									{
										URL:    "http://10.0.0.2:80",
										Scheme: "",
										Port:   "",
									},
								},
							},
						},
					},
				},
			},
			provided: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
			},
			event: message.Message{
				Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "foo",
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "10.1.0.1",
						Ports: []corev1.ServicePort{
							{
								Name:     "test",
								Port:     80,
								Protocol: "TCP",
							},
						},
					},
				},
				Action: message.TypeCreated,
			},
		},
		{
			desc:     "endpoints error",
			mockFile: "build_configuration_simple.yaml",
			expected: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
			},
			provided: &config.Configuration{
				HTTP: &config.HTTPConfiguration{
					Routers:  map[string]*config.Router{},
					Services: map[string]*config.Service{},
				},
			},

			endpointsError: true,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			clientMock := k8s.NewCoreV1ClientMock(test.mockFile)
			if test.endpointsError {
				clientMock.EnableEndpointsError()
			}

			provider := New(clientMock)
			provider.BuildConfiguration(test.event, test.provided)
			assert.Equal(t, test.expected, test.provided)
		})
	}
}

func TestBuildService(t *testing.T) {
	testCases := []struct {
		desc      string
		mockFile  string
		endpoints *corev1.Endpoints
		expected  *config.Service
	}{
		{
			desc:     "two successful endpoints",
			mockFile: "build_service_simple.yaml",
			endpoints: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "foo",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: "10.0.0.1",
							},
							{
								IP: "10.0.0.2",
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Port: 80,
							},
						},
					},
				},
			},
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
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			clientMock := k8s.NewCoreV1ClientMock(test.mockFile)
			provider := New(clientMock)
			actual := provider.buildService(test.endpoints)
			assert.Equal(t, test.expected, actual)

		})
	}
}
