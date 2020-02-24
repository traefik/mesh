package kubernetes

import (
	"context"
	"testing"

	"github.com/containous/maesh/pkg/providers/base"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

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
	provider := New(k8s.ServiceTypeHTTP, nil, ignored, serviceLister, endpointsLister)

	name := "test"
	namespace := "foo"
	ip := "10.0.0.1"
	port := 80
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
	provider := New(k8s.ServiceTypeHTTP, nil, ignored, serviceLister, endpointsLister)

	port := 10000
	associatedService := "bar"

	actual := provider.buildTCPRouter(port, associatedService)
	assert.Equal(t, expected, actual)
}

func TestBuildConfiguration(t *testing.T) {
	stateTable := &k8s.State{
		Table: map[int]*k8s.ServiceWithPort{
			10000: {
				Name:      "test",
				Namespace: "foo",
				Port:      80,
			},
			10001: {
				Name:      "test",
				Namespace: "foo",
				Port:      443,
			},
		},
	}

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
			desc:     "simple configuration build with multiple port service",
			mockFile: "build_configuration_multiple_ports.yaml",
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
						"test-foo-443-92bb68bb9ffcb54d": {
							EntryPoints: []string{"http-5001"},
							Service:     "test-foo-443-92bb68bb9ffcb54d",
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
						"test-foo-443-92bb68bb9ffcb54d": {
							LoadBalancer: &dynamic.ServersLoadBalancer{
								PassHostHeader: base.Bool(true),
								Servers: []dynamic.Server{
									{
										URL:    "http://10.0.0.3:443",
										Scheme: "",
										Port:   "",
									},
									{
										URL:    "http://10.0.0.4:443",
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
			desc:     "simple configuration build with multiple port TCP service",
			mockFile: "build_configuration_multiple_ports_tcp.yaml",
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
						"test-foo-443-92bb68bb9ffcb54d": {
							EntryPoints: []string{"tcp-10001"},
							Service:     "test-foo-443-92bb68bb9ffcb54d",
							Rule:        "HostSNI(`*`)",
						},
					},
					Services: map[string]*dynamic.TCPService{
						"test-foo-80-6653beb49ee354ea": {
							LoadBalancer: &dynamic.TCPServersLoadBalancer{
								Servers: []dynamic.TCPServer{
									{
										Address: "10.0.0.1:80",
										Port:    "",
									},
									{
										Address: "10.0.0.2:80",
										Port:    "",
									},
								},
							},
						},
						"test-foo-443-92bb68bb9ffcb54d": {
							LoadBalancer: &dynamic.TCPServersLoadBalancer{
								Servers: []dynamic.TCPServer{
									{
										Address: "10.0.0.3:443",
										Port:    "",
									},
									{
										Address: "10.0.0.4:443",
										Port:    "",
									},
								},
							},
						},
					},
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
			provider := New(k8s.ServiceTypeHTTP, stateTable, ignored, clientMock.ServiceLister, clientMock.EndpointsLister)
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
		{
			desc:     "Multiple Ports",
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
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: "10.0.0.3",
							},
							{
								IP: "10.0.0.4",
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Port: 443,
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
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clientMock := k8s.NewClientMock(ctx.Done(), test.mockFile, false)
			ignored := k8s.NewIgnored()
			provider := New(k8s.ServiceTypeHTTP, nil, ignored, clientMock.ServiceLister, clientMock.EndpointsLister)
			actual := provider.buildService(test.endpoints, test.scheme, 80)

			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestBuildTCPService(t *testing.T) {
	stateTable := &k8s.State{
		Table: map[int]*k8s.ServiceWithPort{
			10000: {
				Name:      "test",
				Namespace: "foo",
				Port:      80,
			},
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
		{
			desc:     "Multiple ports",
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
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: "10.0.0.3",
							},
							{
								IP: "10.0.0.4",
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Port: 443,
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
			provider := New(k8s.ServiceTypeHTTP, stateTable, ignored, clientMock.ServiceLister, clientMock.EndpointsLister)
			actual := provider.buildTCPService(test.endpoints, 80)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestGetMeshPort(t *testing.T) {
	stateTable := &k8s.State{
		Table: map[int]*k8s.ServiceWithPort{
			10000: {
				Name:      "foo",
				Namespace: "bar",
				Port:      80,
			},
		},
	}

	testCases := []struct {
		desc      string
		name      string
		namespace string
		port      int32
		expected  int
	}{
		{
			desc:      "match in state table",
			name:      "foo",
			namespace: "bar",
			port:      80,
			expected:  10000,
		},
		{
			desc:      "no match in state table",
			name:      "floo",
			namespace: "floo",
			port:      80,
			expected:  0,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			ignored := k8s.NewIgnored()

			fakeClient := fake.NewSimpleClientset()
			kubernetesFactory := informers.NewSharedInformerFactoryWithOptions(fakeClient, k8s.ResyncPeriod)
			serviceLister := kubernetesFactory.Core().V1().Services().Lister()
			endpointsLister := kubernetesFactory.Core().V1().Endpoints().Lister()
			provider := New(k8s.ServiceTypeHTTP, stateTable, ignored, serviceLister, endpointsLister)
			actual := provider.getMeshPort(test.name, test.namespace, test.port)
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
			provider := New(k8s.ServiceTypeHTTP, nil, k8s.NewIgnored(), serviceLister, endpointsLister)
			actual := provider.buildHTTPMiddlewares(test.annotations)
			assert.Equal(t, test.expected, actual)
		})
	}
}
