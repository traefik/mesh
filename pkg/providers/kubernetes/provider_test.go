package kubernetes

import (
	"context"
	"testing"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/providers/base"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

type tcpMappingPortMock func(svc k8s.ServiceWithPort) (int32, bool)

func (t tcpMappingPortMock) Find(svc k8s.ServiceWithPort) (int32, bool) {
	return t(svc)
}

func TestBuildRouter(t *testing.T) {
	expectedWithMiddlewares := &dynamic.Router{
		Rule:        "Host(`test.foo.maesh`) || Host(`10.0.0.1`)",
		EntryPoints: []string{"http-80"},
		Middlewares: []string{"bar"},
		Service:     "bar",
	}

	expectedWithoutMiddlewares := &dynamic.Router{
		Rule:        "Host(`test.foo.maesh`) || Host(`10.0.0.1`)",
		EntryPoints: []string{"http-80"},
		Service:     "bar",
	}

	ignored := k8s.NewIgnored()
	fakeClient := fake.NewSimpleClientset()
	kubernetesFactory := informers.NewSharedInformerFactoryWithOptions(fakeClient, k8s.ResyncPeriod)
	serviceLister := kubernetesFactory.Core().V1().Services().Lister()
	endpointsLister := kubernetesFactory.Core().V1().Endpoints().Lister()
	provider := New(k8s.ServiceTypeHTTP, nil, ignored, serviceLister, endpointsLister, 5000, 50100)

	name := "test"
	namespace := "foo"
	ip := "10.0.0.1"
	port := int32(80)
	associatedService := "bar"

	actual := provider.buildRouter(name, namespace, ip, port, associatedService, true)
	assert.Equal(t, expectedWithMiddlewares, actual)
	actual = provider.buildRouter(name, namespace, ip, port, associatedService, false)
	assert.Equal(t, expectedWithoutMiddlewares, actual)
}

func TestBuildTCPRouter(t *testing.T) {
	expected := &dynamic.TCPRouter{
		Rule:        "HostSNI(`*`)",
		EntryPoints: []string{"tcp-10000"},
		Service:     "bar",
	}

	ignored := k8s.NewIgnored()

	fakeClient := fake.NewSimpleClientset()
	kubernetesFactory := informers.NewSharedInformerFactoryWithOptions(fakeClient, k8s.ResyncPeriod)
	serviceLister := kubernetesFactory.Core().V1().Services().Lister()
	endpointsLister := kubernetesFactory.Core().V1().Endpoints().Lister()
	provider := New(k8s.ServiceTypeHTTP, nil, ignored, serviceLister, endpointsLister, 5000, 50100)

	port := int32(10000)
	associatedService := "bar"

	actual := provider.buildTCPRouter(port, associatedService)
	assert.Equal(t, expected, actual)
}

func TestBuildConfiguration(t *testing.T) {
	testCases := []struct {
		desc           string
		mockFile       string
		expected       *dynamic.Configuration
		endpointsError bool
		serviceError   bool
	}{
		{
			desc:     "simple configuration build with HTTP service",
			mockFile: "build_configuration_simple.yaml",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers: map[string]*dynamic.Router{
						"readiness": {
							EntryPoints: []string{"readiness"},
							Service:     "readiness",
							Rule:        "Path(`/ping`)",
						},
						"test-foo-80-6653beb49ee354ea": {
							EntryPoints: []string{"http-5000"},
							Service:     "test-foo-80-6653beb49ee354ea",
							Rule:        "Host(`test.foo.maesh`) || Host(`10.1.0.1`)",
						},
					},
					Middlewares: map[string]*dynamic.Middleware{},
					Services: map[string]*dynamic.Service{
						"readiness": {
							LoadBalancer: &dynamic.ServersLoadBalancer{
								PassHostHeader: base.Bool(true),
								Servers: []dynamic.Server{
									{
										URL:    "http://127.0.0.1:8080",
										Scheme: "",
										Port:   "",
									},
								},
							},
						},
						"test-foo-80-6653beb49ee354ea": {
							LoadBalancer: &dynamic.ServersLoadBalancer{
								PassHostHeader: base.Bool(true),
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
		},
		{
			desc:     "simple configuration build with HTTP service middlewares",
			mockFile: "build_configuration_simple_http_middlewares.yaml",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers: map[string]*dynamic.Router{
						"readiness": {
							EntryPoints: []string{"readiness"},
							Service:     "readiness",
							Rule:        "Path(`/ping`)",
						},
						"test-foo-80-6653beb49ee354ea": {
							EntryPoints: []string{"http-5000"},
							Middlewares: []string{"test-foo-80-6653beb49ee354ea"},
							Service:     "test-foo-80-6653beb49ee354ea",
							Rule:        "Host(`test.foo.maesh`) || Host(`10.1.0.1`)",
						},
					},
					Middlewares: map[string]*dynamic.Middleware{
						"test-foo-80-6653beb49ee354ea": {
							Retry: &dynamic.Retry{
								Attempts: 2,
							},
						},
					},
					Services: map[string]*dynamic.Service{
						"readiness": {
							LoadBalancer: &dynamic.ServersLoadBalancer{
								PassHostHeader: base.Bool(true),
								Servers: []dynamic.Server{
									{
										URL:    "http://127.0.0.1:8080",
										Scheme: "",
										Port:   "",
									},
								},
							},
						},
						"test-foo-80-6653beb49ee354ea": {
							LoadBalancer: &dynamic.ServersLoadBalancer{
								PassHostHeader: base.Bool(true),
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
		},
		{
			desc:     "simple configuration build with TCP service",
			mockFile: "build_configuration_simple_tcp.yaml",
			expected: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers: map[string]*dynamic.Router{
						"readiness": {
							EntryPoints: []string{"readiness"},
							Service:     "readiness",
							Rule:        "Path(`/ping`)",
						},
					},
					Middlewares: map[string]*dynamic.Middleware{},
					Services: map[string]*dynamic.Service{
						"readiness": {
							LoadBalancer: &dynamic.ServersLoadBalancer{
								PassHostHeader: base.Bool(true),
								Servers: []dynamic.Server{
									{
										URL:    "http://127.0.0.1:8080",
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
						"test-foo-80-6653beb49ee354ea": {
							EntryPoints: []string{"tcp-10000"},
							Service:     "test-foo-80-6653beb49ee354ea",
							Rule:        "HostSNI(`*`)",
						},
					},
					Services: map[string]*dynamic.TCPService{
						"test-foo-80-6653beb49ee354ea": {
							LoadBalancer: &dynamic.TCPServersLoadBalancer{
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
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clientMock := k8s.NewClientMock(ctx.Done(), test.mockFile, false)
			ignored := k8s.NewIgnored()

			findTCPPort := func(svc k8s.ServiceWithPort) (int32, bool) {
				service := k8s.ServiceWithPort{
					Namespace: "foo",
					Name:      "test",
					Port:      80,
				}

				if service == svc {
					return 10000, true
				}

				return 0, false
			}

			provider := New(k8s.ServiceTypeHTTP, tcpMappingPortMock(findTCPPort), ignored, clientMock.ServiceLister, clientMock.EndpointsLister, 5000, 50100)
			config, err := provider.BuildConfig()
			assert.NoError(t, err)

			assert.Equal(t, test.expected, config)
			if test.endpointsError || test.serviceError {
				assert.Error(t, err)
			}
		})
	}
}

func TestBuildService(t *testing.T) {
	testCases := []struct {
		desc      string
		mockFile  string
		endpoints *corev1.Endpoints
		scheme    string
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
			scheme: "http",
			expected: &dynamic.Service{
				LoadBalancer: &dynamic.ServersLoadBalancer{
					PassHostHeader: base.Bool(true),
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
		{
			desc:     "two successful h2c endpoints",
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
			scheme: "h2c",
			expected: &dynamic.Service{
				LoadBalancer: &dynamic.ServersLoadBalancer{
					PassHostHeader: base.Bool(true),
					Servers: []dynamic.Server{
						{
							URL: "h2c://10.0.0.1:80",
						},
						{
							URL: "h2c://10.0.0.2:80",
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

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clientMock := k8s.NewClientMock(ctx.Done(), test.mockFile, false)
			ignored := k8s.NewIgnored()
			provider := New(k8s.ServiceTypeHTTP, nil, ignored, clientMock.ServiceLister, clientMock.EndpointsLister, 5000, 50100)
			actual := provider.buildService(test.endpoints, test.scheme)

			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestBuildTCPService(t *testing.T) {
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
				LoadBalancer: &dynamic.TCPServersLoadBalancer{
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

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clientMock := k8s.NewClientMock(ctx.Done(), test.mockFile, false)
			ignored := k8s.NewIgnored()

			findTCPPort := func(svc k8s.ServiceWithPort) (int32, bool) {
				service := k8s.ServiceWithPort{
					Namespace: "foo",
					Name:      "test",
					Port:      80,
				}

				if service == svc {
					return 10000, true
				}

				return 0, false
			}

			provider := New(k8s.ServiceTypeHTTP, tcpMappingPortMock(findTCPPort), ignored, clientMock.ServiceLister, clientMock.EndpointsLister, 5000, 50100)
			actual := provider.buildTCPService(test.endpoints)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestBuildHTTPMiddlewares(t *testing.T) {
	testCases := []struct {
		desc        string
		annotations map[string]string
		expected    *dynamic.Middleware
	}{
		{
			desc:        "empty annotations",
			annotations: map[string]string{},
			expected:    nil,
		},
		{
			desc: "Parsable retry",
			annotations: map[string]string{
				k8s.AnnotationRetryAttempts: "2",
			},
			expected: &dynamic.Middleware{
				Retry: &dynamic.Retry{
					Attempts: 2,
				},
			},
		},
		{
			desc: "unparsable retry",
			annotations: map[string]string{
				k8s.AnnotationRetryAttempts: "abc",
			},
			expected: nil,
		},
		{
			desc: "existing cb expression",
			annotations: map[string]string{
				k8s.AnnotationCircuitBreakerExpression: "toto",
			},
			expected: &dynamic.Middleware{
				CircuitBreaker: &dynamic.CircuitBreaker{
					Expression: "toto",
				},
			},
		},
		{
			desc: "empty cb expression",
			annotations: map[string]string{
				k8s.AnnotationCircuitBreakerExpression: "",
			},
			expected: nil,
		},
		{
			desc: "parseable rate limit",
			annotations: map[string]string{
				k8s.AnnotationRateLimitAverage: "100",
				k8s.AnnotationRateLimitBurst:   "200",
			},
			expected: &dynamic.Middleware{
				RateLimit: &dynamic.RateLimit{
					Average: 100,
					Burst:   200,
				},
			},
		},
		{
			desc: "empty rate limit",
			annotations: map[string]string{
				k8s.AnnotationRateLimitAverage: "",
				k8s.AnnotationRateLimitBurst:   "",
			},
			expected: nil,
		},
		{
			desc: "unparseable rate limit",
			annotations: map[string]string{
				k8s.AnnotationRateLimitAverage: "foo",
				k8s.AnnotationRateLimitBurst:   "bar",
			},
			expected: nil,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			fakeClient := fake.NewSimpleClientset()
			kubernetesFactory := informers.NewSharedInformerFactoryWithOptions(fakeClient, k8s.ResyncPeriod)
			serviceLister := kubernetesFactory.Core().V1().Services().Lister()
			endpointsLister := kubernetesFactory.Core().V1().Endpoints().Lister()
			provider := New(k8s.ServiceTypeHTTP, nil, k8s.NewIgnored(), serviceLister, endpointsLister, 5000, 50100)
			actual := provider.buildHTTPMiddlewares(test.annotations)
			assert.Equal(t, test.expected, actual)
		})
	}
}
