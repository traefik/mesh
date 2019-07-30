package kubernetes

import (
	"testing"

	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/maesh/internal/message"
	"github.com/containous/traefik/pkg/config/dynamic"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const meshNamespace string = "maesh"

func TestBuildRouter(t *testing.T) {
	expected := &dynamic.Router{
		Rule:        "Host(`test.foo.maesh`) || Host(`10.0.0.1`)",
		EntryPoints: []string{"http-80"},
		Service:     "bar",
	}

	provider := New(nil, k8s.ServiceTypeHTTP, meshNamespace, nil)

	name := "test"
	namespace := "foo"
	ip := "10.0.0.1"
	port := 80
	associatedService := "bar"

	actual := provider.buildRouter(name, namespace, ip, port, associatedService)
	assert.Equal(t, expected, actual)
}

func TestBuildTCPRouter(t *testing.T) {
	expected := &dynamic.TCPRouter{
		Rule:        "HostSNI(`*`)",
		EntryPoints: []string{"tcp-10000"},
		Service:     "bar",
	}

	provider := New(nil, k8s.ServiceTypeTCP, meshNamespace, nil)

	port := 10000
	associatedService := "bar"

	actual := provider.buildTCPRouter(port, associatedService)
	assert.Equal(t, expected, actual)

}

func TestBuildConfiguration(t *testing.T) {
	stateTable := map[int]k8s.ServiceWithPort{
		10000: {
			Name:      "test",
			Namespace: "foo",
			Port:      80,
		},
	}

	testCases := []struct {
		desc           string
		mockFile       string
		event          message.Message
		provided       *dynamic.Configuration
		expected       *dynamic.Configuration
		endpointsError bool
		serviceError   bool
	}{
		{
			desc:     "simple configuration build with empty event",
			mockFile: "build_configuration_simple.yaml",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:  map[string]*dynamic.Router{},
					Services: map[string]*dynamic.Service{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:  map[string]*dynamic.Router{},
					Services: map[string]*dynamic.Service{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
		},
		{
			desc:     "simple configuration build with HTTP service event",
			mockFile: "build_configuration_simple.yaml",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers: map[string]*dynamic.Router{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							EntryPoints: []string{"http-5000"},
							Service:     "6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839",
							Rule:        "Host(`test.foo.maesh`) || Host(`10.1.0.1`)",
						},
					},
					Services: map[string]*dynamic.Service{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							LoadBalancer: &dynamic.LoadBalancerService{
								PassHostHeader: true,
								Servers: []dynamic.Server{
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
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:  map[string]*dynamic.Router{},
					Services: map[string]*dynamic.Service{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
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
			desc:     "simple configuration build with TCP service event",
			mockFile: "build_configuration_simple.yaml",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:  map[string]*dynamic.Router{},
					Services: map[string]*dynamic.Service{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers: map[string]*dynamic.TCPRouter{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							EntryPoints: []string{"tcp-10000"},
							Service:     "6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839",
							Rule:        "HostSNI(`*`)",
						},
					},
					Services: map[string]*dynamic.TCPService{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							LoadBalancer: &dynamic.TCPLoadBalancerService{
								Servers: []dynamic.TCPServer{
									{
										Address: "10.0.0.1:80",
									},
									{
										Address: "10.0.0.2:80",
									},
								},
							},
						},
					},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:  map[string]*dynamic.Router{},
					Services: map[string]*dynamic.Service{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			event: message.Message{
				Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "foo",
						Annotations: map[string]string{
							k8s.AnnotationServiceType: k8s.ServiceTypeTCP,
						},
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
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:  map[string]*dynamic.Router{},
					Services: map[string]*dynamic.Service{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:  map[string]*dynamic.Router{},
					Services: map[string]*dynamic.Service{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			event: message.Message{
				Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "foo",
					},
				},
				Action: message.TypeCreated,
			},

			endpointsError: true,
		},
		{
			desc:     "endpoints not exist error",
			mockFile: "build_configuration_simple.yaml",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:  map[string]*dynamic.Router{},
					Services: map[string]*dynamic.Service{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:  map[string]*dynamic.Router{},
					Services: map[string]*dynamic.Service{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			event: message.Message{
				Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "bar",
					},
				},
				Action: message.TypeCreated,
			},
		},
		{
			desc:     "service error",
			mockFile: "build_configuration_simple.yaml",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:  map[string]*dynamic.Router{},
					Services: map[string]*dynamic.Service{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:  map[string]*dynamic.Router{},
					Services: map[string]*dynamic.Service{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			event: message.Message{
				Object: &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "foo",
					},
				},
				Action: message.TypeUpdated,
			},

			serviceError: true,
		},
		{
			desc:     "service not exist error",
			mockFile: "build_configuration_simple.yaml",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:  map[string]*dynamic.Router{},
					Services: map[string]*dynamic.Service{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:  map[string]*dynamic.Router{},
					Services: map[string]*dynamic.Service{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			event: message.Message{
				Object: &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "bar",
					},
				},
				Action: message.TypeUpdated,
			},
		},
		{
			desc:     "simple configuration delete HTTP service event",
			mockFile: "build_configuration_simple.yaml",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers:  map[string]*dynamic.Router{},
					Services: map[string]*dynamic.Service{},
				},
				TCP: &dynamic.TCPConfiguration{
					Routers: map[string]*dynamic.TCPRouter{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							EntryPoints: []string{"ingress-5000"},
							Service:     "6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839",
							Rule:        "HostSNI(`*`)",
						},
					},
					Services: map[string]*dynamic.TCPService{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							LoadBalancer: &dynamic.TCPLoadBalancerService{
								Servers: []dynamic.TCPServer{
									{
										Address: "10.0.0.1:80",
									},
									{
										Address: "10.0.0.2:80",
									},
								},
							},
						},
					},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers: map[string]*dynamic.Router{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							EntryPoints: []string{"ingress-5000"},
							Service:     "6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839",
							Rule:        "Host(`test.foo.maesh`) || Host(`10.1.0.1`)",
						},
					},
					Services: map[string]*dynamic.Service{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							LoadBalancer: &dynamic.LoadBalancerService{
								PassHostHeader: true,
								Servers: []dynamic.Server{
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
				TCP: &dynamic.TCPConfiguration{
					Routers: map[string]*dynamic.TCPRouter{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							EntryPoints: []string{"ingress-5000"},
							Service:     "6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839",
							Rule:        "HostSNI(`*`)",
						},
					},
					Services: map[string]*dynamic.TCPService{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							LoadBalancer: &dynamic.TCPLoadBalancerService{
								Servers: []dynamic.TCPServer{
									{
										Address: "10.0.0.1:80",
									},
									{
										Address: "10.0.0.2:80",
									},
								},
							},
						},
					},
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
				Action: message.TypeDeleted,
			},
		},
		{
			desc:     "simple configuration delete TCP service event",
			mockFile: "build_configuration_simple.yaml",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers: map[string]*dynamic.Router{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							EntryPoints: []string{"ingress-5000"},
							Service:     "6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839",
							Rule:        "Host(`test.foo.maesh`) || Host(`10.1.0.1`)",
						},
					},
					Services: map[string]*dynamic.Service{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							LoadBalancer: &dynamic.LoadBalancerService{
								PassHostHeader: true,
								Servers: []dynamic.Server{
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
				TCP: &dynamic.TCPConfiguration{
					Routers:  map[string]*dynamic.TCPRouter{},
					Services: map[string]*dynamic.TCPService{},
				},
			},
			provided: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers: map[string]*dynamic.Router{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							EntryPoints: []string{"ingress-5000"},
							Service:     "6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839",
							Rule:        "Host(`test.foo.maesh`) || Host(`10.1.0.1`)",
						},
					},
					Services: map[string]*dynamic.Service{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							LoadBalancer: &dynamic.LoadBalancerService{
								PassHostHeader: true,
								Servers: []dynamic.Server{
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
				TCP: &dynamic.TCPConfiguration{
					Routers: map[string]*dynamic.TCPRouter{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							EntryPoints: []string{"ingress-5000"},
							Service:     "6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839",
							Rule:        "HostSNI(`*`)",
						},
					},
					Services: map[string]*dynamic.TCPService{
						"6653beb49ee354ea9d22028a3816f8947fe6b2f8362e42eb258e884769be2839": {
							LoadBalancer: &dynamic.TCPLoadBalancerService{
								Servers: []dynamic.TCPServer{
									{
										Address: "10.0.0.1:80",
									},
									{
										Address: "10.0.0.2:80",
									},
								},
							},
						},
					},
				},
			},
			event: message.Message{
				Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "foo",
						Annotations: map[string]string{
							k8s.AnnotationServiceType: k8s.ServiceTypeTCP,
						},
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
				Action: message.TypeDeleted,
			},
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
			if test.serviceError {
				clientMock.EnableServiceError()
			}

			provider := New(clientMock, k8s.ServiceTypeHTTP, meshNamespace, &stateTable)
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
		expected  *dynamic.Service
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
			expected: &dynamic.Service{
				LoadBalancer: &dynamic.LoadBalancerService{
					PassHostHeader: true,
					Servers: []dynamic.Server{
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
			provider := New(clientMock, k8s.ServiceTypeHTTP, meshNamespace, nil)
			actual := provider.buildService(test.endpoints)
			assert.Equal(t, test.expected, actual)

		})
	}
}

func TestBuildTCPService(t *testing.T) {
	stateTable := map[int]k8s.ServiceWithPort{
		10000: {
			Name:      "test",
			Namespace: "foo",
			Port:      80,
		},
	}

	testCases := []struct {
		desc      string
		mockFile  string
		endpoints *corev1.Endpoints
		expected  *dynamic.TCPService
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
			expected: &dynamic.TCPService{
				LoadBalancer: &dynamic.TCPLoadBalancerService{
					Servers: []dynamic.TCPServer{
						{
							Address: "10.0.0.1:80",
						},
						{
							Address: "10.0.0.2:80",
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
			provider := New(clientMock, k8s.ServiceTypeHTTP, meshNamespace, &stateTable)
			actual := provider.buildTCPService(test.endpoints)
			assert.Equal(t, test.expected, actual)

		})
	}
}
