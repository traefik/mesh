package provider_test

import (
	"io/ioutil"
	"testing"

	mk8s "github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/provider"
	"github.com/containous/maesh/pkg/topology"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	spec "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha1"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type TopologyBuilderMock func() (*topology.Topology, error)

func (m TopologyBuilderMock) Build(_ mk8s.IgnoreWrapper) (*topology.Topology, error) {
	return m()
}

type tcpStateTableMock func(svcPort mk8s.ServiceWithPort) (int32, bool)

func (t tcpStateTableMock) Find(svcPort mk8s.ServiceWithPort) (int32, bool) {
	return t(svcPort)
}

func TestProvider_BuildConfigWithACLDisabled(t *testing.T) {
	annotations := map[string]string{}
	svcbAnnotations := map[string]string{
		"maesh.containo.us/scheme":                     "https",
		"maesh.containo.us/retry-attempts":             "2",
		"maesh.containo.us/ratelimit-average":          "100",
		"maesh.containo.us/ratelimit-burst":            "110",
		"maesh.containo.us/circuit-breaker-expression": "LatencyAtQuantileMS(50.0) > 100",
	}
	svcfAnnotations := map[string]string{
		"maesh.containo.us/traffic-type": "tcp",
	}
	ports := []v1.ServicePort{svcPort("port-8080", 8080, 8080)}
	saB := "sa-b"

	podB1 := createPod("my-ns", "pod-b1", saB, "10.10.2.1")
	podB2 := createPod("my-ns", "pod-b2", saB, "10.10.2.2")
	svcB := createSvc("my-ns", "svc-b", svcbAnnotations, ports, "10.10.14.1", []*topology.Pod{podB1, podB2})
	svcD := createSvc("my-ns", "svc-d", annotations, ports, "10.10.15.1", []*topology.Pod{})
	svcE := createSvc("my-ns", "svc-e", annotations, ports, "10.10.16.1", []*topology.Pod{})
	svcF := createSvc("my-ns", "svc-f", svcfAnnotations, ports, "10.10.17.1", []*topology.Pod{podB1, podB2})

	backendSvcD := createTrafficSplitBackend(svcD, 80)
	backendSvcE := createTrafficSplitBackend(svcE, 20)
	ts := createTrafficSplit("my-ns", "ts", svcB, []topology.TrafficSplitBackend{backendSvcD, backendSvcE}, nil)

	svcD.BackendOf = []*topology.TrafficSplit{ts}
	svcE.BackendOf = []*topology.TrafficSplit{ts}
	svcB.TrafficSplits = []*topology.TrafficSplit{ts}

	top := &topology.Topology{
		Services: map[topology.NameNamespace]*topology.Service{
			nn(svcB.Name, svcB.Namespace): svcB,
			nn(svcD.Name, svcD.Namespace): svcD,
			nn(svcE.Name, svcE.Namespace): svcE,
			nn(svcF.Name, svcF.Namespace): svcF,
		},
		Pods: map[topology.NameNamespace]*topology.Pod{
			nn(podB1.Name, podB1.Namespace): podB1,
			nn(podB2.Name, podB2.Namespace): podB2,
		},
	}
	builder := func() (*topology.Topology, error) {
		return top, nil
	}

	tcpStatetable := func(_ mk8s.ServiceWithPort) (int32, bool) {
		return 5000, true
	}

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	ignored := mk8s.NewIgnored()
	p := provider.New(TopologyBuilderMock(builder), tcpStateTableMock(tcpStatetable), ignored, 10000, 10001, false, "http", "maesh", logger)

	got, err := p.BuildConfig()
	require.NoError(t, err)

	want := &dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{
			Routers: map[string]*dynamic.Router{
				"readiness": readinessRtr,
				"my-ns-svc-b-8080": {
					Rule:        "Host(`svc-b.my-ns.maesh`) || Host(`10.10.14.1`)",
					EntryPoints: []string{"http-10000"},
					Service:     "my-ns-svc-b-8080",
					Priority:    1001,
					Middlewares: []string{"my-ns-svc-b"},
				},
				"my-ns-svc-b-ts-8080-traffic-split-direct": {
					Rule:        "Host(`svc-b.my-ns.maesh`) || Host(`10.10.14.1`)",
					EntryPoints: []string{"http-10000"},
					Service:     "my-ns-svc-b-ts-8080-traffic-split",
					Priority:    4001,
					Middlewares: []string{"my-ns-svc-b"},
				},
				"my-ns-svc-d-8080": {
					Rule:        "Host(`svc-d.my-ns.maesh`) || Host(`10.10.15.1`)",
					EntryPoints: []string{"http-10000"},
					Priority:    1001,
					Service:     "my-ns-svc-d-8080",
				},
				"my-ns-svc-e-8080": {
					Rule:        "Host(`svc-e.my-ns.maesh`) || Host(`10.10.16.1`)",
					EntryPoints: []string{"http-10000"},
					Priority:    1001,
					Service:     "my-ns-svc-e-8080",
				},
			},
			Services: map[string]*dynamic.Service{
				"block-all-service": blockAllService,
				"readiness":         readinessSvc,
				"my-ns-svc-b-8080": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						Servers: []dynamic.Server{
							{URL: "https://10.10.2.1:8080"},
							{URL: "https://10.10.2.2:8080"},
						},
						PassHostHeader: getBoolRef(true),
					},
				},
				"my-ns-svc-b-ts-8080-traffic-split": {
					Weighted: &dynamic.WeightedRoundRobin{
						Services: []dynamic.WRRService{
							{
								Name:   "my-ns-svc-b-ts-8080-svc-d-traffic-split-backend",
								Weight: getIntRef(80),
							},
							{
								Name:   "my-ns-svc-b-ts-8080-svc-e-traffic-split-backend",
								Weight: getIntRef(20),
							},
						},
					},
				},
				"my-ns-svc-b-ts-8080-svc-d-traffic-split-backend": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						Servers: []dynamic.Server{
							{URL: "https://svc-d.my-ns.maesh:8080"},
						},
						PassHostHeader: getBoolRef(false),
					},
				},
				"my-ns-svc-b-ts-8080-svc-e-traffic-split-backend": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						Servers: []dynamic.Server{
							{URL: "https://svc-e.my-ns.maesh:8080"},
						},
						PassHostHeader: getBoolRef(false),
					},
				},
				"my-ns-svc-d-8080": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						PassHostHeader: getBoolRef(true),
					},
				},
				"my-ns-svc-e-8080": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						PassHostHeader: getBoolRef(true),
					},
				},
			},
			Middlewares: map[string]*dynamic.Middleware{
				"block-all-middleware": blockAllMiddleware,
				"my-ns-svc-b": {
					Retry: &dynamic.Retry{Attempts: 2},
					RateLimit: &dynamic.RateLimit{
						Average: 100,
						Burst:   110,
					},
					CircuitBreaker: &dynamic.CircuitBreaker{
						Expression: "LatencyAtQuantileMS(50.0) > 100",
					},
				},
			},
		},
		TCP: &dynamic.TCPConfiguration{
			Routers: map[string]*dynamic.TCPRouter{
				"my-ns-svc-f-8080": {
					EntryPoints: []string{"tcp-5000"},
					Service:     "my-ns-svc-f-8080",
					Rule:        "HostSNI(`*`)",
				},
			},
			Services: map[string]*dynamic.TCPService{
				"my-ns-svc-f-8080": {
					LoadBalancer: &dynamic.TCPServersLoadBalancer{
						Servers: []dynamic.TCPServer{
							{
								Address: "10.10.2.1:8080",
							},
							{
								Address: "10.10.2.2:8080",
							},
						},
					},
				},
			},
		},
	}

	assert.Equal(t, want, got)
}

func TestProvider_BuildConfigTCP(t *testing.T) {
	saA := "sa-a"
	saB := "sa-b"
	ports := []v1.ServicePort{svcPort("port-8080", 8080, 8080)}
	annotations := map[string]string{}

	podA := createPod("my-ns", "pod-a", saA, "10.10.1.1")
	podB := createPod("my-ns", "pod-b", saB, "10.10.1.2")
	podC := createPod("my-ns", "pod-c", saB, "10.10.1.3")
	podD := createPod("my-ns", "pod-d", saB, "10.10.1.4")
	svcB := createSvc("my-ns", "svc-b", annotations, ports, "10.10.13.1", []*topology.Pod{podB})
	svcC := createSvc("my-ns", "svc-c", annotations, ports, "10.10.13.2", []*topology.Pod{podC})
	svcD := createSvc("my-ns", "svc-d", annotations, ports, "10.10.13.3", []*topology.Pod{podD})

	backendSvcC := createTrafficSplitBackend(svcC, 80)
	backendSvcD := createTrafficSplitBackend(svcD, 20)
	ts := createTrafficSplit("my-ns", "ts", svcB, []topology.TrafficSplitBackend{backendSvcC, backendSvcD}, []*topology.Pod{podA})

	svcC.BackendOf = []*topology.TrafficSplit{ts}
	svcD.BackendOf = []*topology.TrafficSplit{ts}

	sourceA := createTrafficTargetSource("my-ns", saA, []*topology.Pod{podA})
	destSvcB := createTrafficTargetDestination("my-ns", saB, []*topology.Pod{podB}, ports)
	spec := topology.TrafficSpec{TCPRoute: createTCPRoute("my-ns", "my-tcp-route")}
	ttSvcB := createTrafficTarget("tt", svcB, []topology.ServiceTrafficTargetSource{sourceA}, destSvcB, []topology.TrafficSpec{spec})
	destSvcC := createTrafficTargetDestination("my-ns", saB, []*topology.Pod{podC}, ports)
	ttSvcC := createTrafficTarget("tt", svcC, []topology.ServiceTrafficTargetSource{sourceA}, destSvcC, []topology.TrafficSpec{spec})
	destSvcD := createTrafficTargetDestination("my-ns", saB, []*topology.Pod{podD}, ports)
	ttSvcD := createTrafficTarget("tt", svcD, []topology.ServiceTrafficTargetSource{sourceA}, destSvcD, []topology.TrafficSpec{spec})

	podA.Outgoing = []*topology.ServiceTrafficTarget{ttSvcB, ttSvcC, ttSvcD}
	podB.Incoming = []*topology.ServiceTrafficTarget{ttSvcB}
	podC.Incoming = []*topology.ServiceTrafficTarget{ttSvcC}
	podD.Incoming = []*topology.ServiceTrafficTarget{ttSvcD}
	svcB.TrafficTargets = []*topology.ServiceTrafficTarget{ttSvcB}
	svcC.TrafficTargets = []*topology.ServiceTrafficTarget{ttSvcC}
	svcD.TrafficTargets = []*topology.ServiceTrafficTarget{ttSvcD}
	svcB.TrafficSplits = []*topology.TrafficSplit{ts}

	top := &topology.Topology{
		Services: map[topology.NameNamespace]*topology.Service{
			nn(svcB.Name, svcB.Namespace): svcB,
			nn(svcC.Name, svcC.Namespace): svcC,
			nn(svcD.Name, svcD.Namespace): svcD,
		},
		Pods: map[topology.NameNamespace]*topology.Pod{
			nn(podA.Name, podA.Namespace): podA,
			nn(podB.Name, podB.Namespace): podB,
			nn(podC.Name, podC.Namespace): podC,
			nn(podD.Name, podD.Namespace): podD,
		},
	}
	builder := func() (*topology.Topology, error) {
		return top, nil
	}

	tcpStatetable := func(svcPort mk8s.ServiceWithPort) (int32, bool) {
		switch svcPort.Name {
		case svcB.Name:
			return 5000, true
		case svcC.Name:
			return 5001, true
		case svcD.Name:
			return 5002, true
		}

		return 5003, true
	}

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	ignored := mk8s.NewIgnored()
	p := provider.New(TopologyBuilderMock(builder), tcpStateTableMock(tcpStatetable), ignored, 10000, 10001, true, "tcp", "maesh", logger)

	got, err := p.BuildConfig()
	require.NoError(t, err)

	want := &dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{
			Routers: map[string]*dynamic.Router{
				"readiness": readinessRtr,
			},
			Services: map[string]*dynamic.Service{
				"readiness":         readinessSvc,
				"block-all-service": blockAllService,
			},
			Middlewares: map[string]*dynamic.Middleware{
				"block-all-middleware": blockAllMiddleware,
			},
		},
		TCP: &dynamic.TCPConfiguration{
			Routers: map[string]*dynamic.TCPRouter{
				"my-ns-svc-b-8080": {
					EntryPoints: []string{"tcp-5000"},
					Service:     "my-ns-svc-b-8080",
					Rule:        "HostSNI(`*`)",
				},
				"my-ns-svc-c-8080": {
					EntryPoints: []string{"tcp-5001"},
					Service:     "my-ns-svc-c-8080",
					Rule:        "HostSNI(`*`)",
				},
				"my-ns-svc-d-8080": {
					EntryPoints: []string{"tcp-5002"},
					Service:     "my-ns-svc-d-8080",
					Rule:        "HostSNI(`*`)",
				},
			},
			Services: map[string]*dynamic.TCPService{
				"my-ns-svc-b-8080": {
					Weighted: &dynamic.TCPWeightedRoundRobin{
						Services: []dynamic.TCPWRRService{
							{
								Name:   "my-ns-svc-b-ts-8080-svc-c-traffic-split-backend",
								Weight: getIntRef(80),
							},
							{
								Name:   "my-ns-svc-b-ts-8080-svc-d-traffic-split-backend",
								Weight: getIntRef(20),
							},
						},
					},
				},
				"my-ns-svc-b-ts-8080-svc-c-traffic-split-backend": {
					LoadBalancer: &dynamic.TCPServersLoadBalancer{
						Servers: []dynamic.TCPServer{
							{
								Address: "svc-c.my-ns.maesh:8080",
							},
						},
					},
				},
				"my-ns-svc-b-ts-8080-svc-d-traffic-split-backend": {
					LoadBalancer: &dynamic.TCPServersLoadBalancer{
						Servers: []dynamic.TCPServer{
							{
								Address: "svc-d.my-ns.maesh:8080",
							},
						},
					},
				},
				"my-ns-svc-c-8080": {
					LoadBalancer: &dynamic.TCPServersLoadBalancer{
						Servers: []dynamic.TCPServer{
							{
								Address: "10.10.1.3:8080",
							},
						},
					},
				},
				"my-ns-svc-d-8080": {
					LoadBalancer: &dynamic.TCPServersLoadBalancer{
						Servers: []dynamic.TCPServer{
							{
								Address: "10.10.1.4:8080",
							},
						},
					},
				},
			},
		},
	}

	assert.Equal(t, want, got)
}

func TestProvider_BuildConfigHTTP(t *testing.T) {
	annotations := map[string]string{}
	svcbAnnotations := map[string]string{
		"maesh.containo.us/retry-attempts":             "2",
		"maesh.containo.us/ratelimit-average":          "100",
		"maesh.containo.us/ratelimit-burst":            "110",
		"maesh.containo.us/circuit-breaker-expression": "LatencyAtQuantileMS(50.0) > 100",
	}
	ports := []v1.ServicePort{svcPort("port-8080", 8080, 8080)}
	saA := "sa-a"
	saB := "sa-b"
	saC := "sa-c"

	podA := createPod("my-ns", "pod-a", saA, "10.10.1.1")
	podC := createPod("my-ns", "pod-c", saC, "10.10.3.1")
	podB1 := createPod("my-ns", "pod-b1", saB, "10.10.2.1")
	podB2 := createPod("my-ns", "pod-b2", saB, "10.10.2.2")
	podD := createPod("my-ns", "pod-d", saB, "10.10.4.1")
	podE := createPod("my-ns", "pod-e", saB, "10.10.5.1")

	svcB := createSvc("my-ns", "svc-b", svcbAnnotations, ports, "10.10.14.1", []*topology.Pod{podB1, podB2})
	svcD := createSvc("my-ns", "svc-d", annotations, ports, "10.10.15.1", []*topology.Pod{podD})
	svcE := createSvc("my-ns", "svc-e", annotations, ports, "10.10.16.1", []*topology.Pod{podE})

	backendSvcD := createTrafficSplitBackend(svcD, 80)
	backendSvcE := createTrafficSplitBackend(svcE, 20)
	ts := createTrafficSplit("my-ns", "ts", svcB, []topology.TrafficSplitBackend{backendSvcD, backendSvcE}, []*topology.Pod{podA, podC})

	svcD.BackendOf = []*topology.TrafficSplit{ts}
	svcE.BackendOf = []*topology.TrafficSplit{ts}

	apiMatch := createHTTPMatch("api", []string{"GET"}, "/api")
	metricMatch := createHTTPMatch("metric", []string{"POST"}, "/metric")
	rtGrp := createHTTPRouteGroup("my-ns", "rt-grp", []spec.HTTPMatch{apiMatch, metricMatch})

	sourceA := createTrafficTargetSource("my-ns", saA, []*topology.Pod{podA})
	sourceC := createTrafficTargetSource("my-ns", saC, []*topology.Pod{podC})
	specTt := topology.TrafficSpec{HTTPRouteGroup: rtGrp, HTTPMatches: []*spec.HTTPMatch{&apiMatch, &metricMatch}}

	destSvcB := createTrafficTargetDestination("my-ns", saB, []*topology.Pod{podB1, podB2}, ports)
	ttSvcB := createTrafficTarget("tt", svcB, []topology.ServiceTrafficTargetSource{sourceA, sourceC}, destSvcB, []topology.TrafficSpec{specTt})

	destSvcD := createTrafficTargetDestination("my-ns", saB, []*topology.Pod{podD}, ports)
	ttSvcD := createTrafficTarget("tt", svcD, []topology.ServiceTrafficTargetSource{sourceA, sourceC}, destSvcD, []topology.TrafficSpec{specTt})

	destSvcE := createTrafficTargetDestination("my-ns", saB, []*topology.Pod{podE}, ports)
	ttSvcE := createTrafficTarget("tt", svcE, []topology.ServiceTrafficTargetSource{sourceA, sourceC}, destSvcE, []topology.TrafficSpec{specTt})

	podA.Outgoing = []*topology.ServiceTrafficTarget{ttSvcB, ttSvcD, ttSvcE}
	podC.Outgoing = []*topology.ServiceTrafficTarget{ttSvcB, ttSvcD, ttSvcE}
	podB1.Incoming = []*topology.ServiceTrafficTarget{ttSvcB}
	podB2.Incoming = []*topology.ServiceTrafficTarget{ttSvcB}
	podD.Incoming = []*topology.ServiceTrafficTarget{ttSvcD}
	podE.Incoming = []*topology.ServiceTrafficTarget{ttSvcE}
	svcB.TrafficTargets = []*topology.ServiceTrafficTarget{ttSvcB}
	svcD.TrafficTargets = []*topology.ServiceTrafficTarget{ttSvcD}
	svcE.TrafficTargets = []*topology.ServiceTrafficTarget{ttSvcE}
	svcB.TrafficSplits = []*topology.TrafficSplit{ts}

	top := &topology.Topology{
		Services: map[topology.NameNamespace]*topology.Service{
			nn(svcB.Name, svcB.Namespace): svcB,
			nn(svcD.Name, svcD.Namespace): svcD,
			nn(svcE.Name, svcE.Namespace): svcE,
		},
		Pods: map[topology.NameNamespace]*topology.Pod{
			nn(podA.Name, podA.Namespace):   podA,
			nn(podC.Name, podC.Namespace):   podC,
			nn(podB1.Name, podB1.Namespace): podB1,
			nn(podB2.Name, podB2.Namespace): podB2,
			nn(podD.Name, podD.Namespace):   podD,
			nn(podE.Name, podE.Namespace):   podE,
		},
	}
	builder := func() (*topology.Topology, error) {
		return top, nil
	}

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	ignored := mk8s.NewIgnored()
	p := provider.New(TopologyBuilderMock(builder), nil, ignored, 10000, 10001, true, "http", "maesh", logger)

	got, err := p.BuildConfig()
	require.NoError(t, err)

	want := &dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{
			Routers: map[string]*dynamic.Router{
				"readiness": readinessRtr,
				// Block all routers.
				"my-ns-svc-b-8080": {
					Rule:        "Host(`svc-b.my-ns.maesh`) || Host(`10.10.14.1`)",
					EntryPoints: []string{"http-10000"},
					Service:     "block-all-service",
					Priority:    1,
					Middlewares: []string{"block-all-middleware"},
				},
				"my-ns-svc-d-8080": {
					Rule:        "Host(`svc-d.my-ns.maesh`) || Host(`10.10.15.1`)",
					EntryPoints: []string{"http-10000"},
					Service:     "block-all-service",
					Priority:    1,
					Middlewares: []string{"block-all-middleware"},
				},
				"my-ns-svc-e-8080": {
					Rule:        "Host(`svc-e.my-ns.maesh`) || Host(`10.10.16.1`)",
					EntryPoints: []string{"http-10000"},
					Service:     "block-all-service",
					Priority:    1,
					Middlewares: []string{"block-all-middleware"},
				},
				// Traffic-target routers (direct).
				"my-ns-svc-b-tt-8080-traffic-target-direct": {
					Rule:        "(Host(`svc-b.my-ns.maesh`) || Host(`10.10.14.1`)) && ((PathPrefix(`/{path:api}`) && Method(`GET`)) || (PathPrefix(`/{path:metric}`) && Method(`POST`)))",
					EntryPoints: []string{"http-10000"},
					Service:     "my-ns-svc-b-tt-8080-traffic-target",
					Priority:    2005,
					Middlewares: []string{"my-ns-svc-b", "my-ns-svc-b-tt-whitelist-traffic-target-direct"},
				},
				"my-ns-svc-d-tt-8080-traffic-target-direct": {
					Rule:        "(Host(`svc-d.my-ns.maesh`) || Host(`10.10.15.1`)) && ((PathPrefix(`/{path:api}`) && Method(`GET`)) || (PathPrefix(`/{path:metric}`) && Method(`POST`)))",
					EntryPoints: []string{"http-10000"},
					Service:     "my-ns-svc-d-tt-8080-traffic-target",
					Priority:    2005,
					Middlewares: []string{"my-ns-svc-d-tt-whitelist-traffic-target-direct"},
				},
				"my-ns-svc-e-tt-8080-traffic-target-direct": {
					Rule:        "(Host(`svc-e.my-ns.maesh`) || Host(`10.10.16.1`)) && ((PathPrefix(`/{path:api}`) && Method(`GET`)) || (PathPrefix(`/{path:metric}`) && Method(`POST`)))",
					EntryPoints: []string{"http-10000"},
					Service:     "my-ns-svc-e-tt-8080-traffic-target",
					Priority:    2005,
					Middlewares: []string{"my-ns-svc-e-tt-whitelist-traffic-target-direct"},
				},
				// Traffic-target routers (indirect).
				"my-ns-svc-d-tt-8080-traffic-target-indirect": {
					Rule:        "(Host(`svc-d.my-ns.maesh`) || Host(`10.10.15.1`)) && ((PathPrefix(`/{path:api}`) && Method(`GET`)) || (PathPrefix(`/{path:metric}`) && Method(`POST`))) && HeadersRegexp(`X-Forwarded-For`, `.+`)",
					EntryPoints: []string{"http-10000"},
					Service:     "my-ns-svc-d-tt-8080-traffic-target",
					Priority:    3006,
					Middlewares: []string{"my-ns-svc-d-tt-whitelist-traffic-target-indirect"},
				},
				"my-ns-svc-e-tt-8080-traffic-target-indirect": {
					Rule:        "(Host(`svc-e.my-ns.maesh`) || Host(`10.10.16.1`)) && ((PathPrefix(`/{path:api}`) && Method(`GET`)) || (PathPrefix(`/{path:metric}`) && Method(`POST`))) && HeadersRegexp(`X-Forwarded-For`, `.+`)",
					EntryPoints: []string{"http-10000"},
					Service:     "my-ns-svc-e-tt-8080-traffic-target",
					Priority:    3006,
					Middlewares: []string{"my-ns-svc-e-tt-whitelist-traffic-target-indirect"},
				},
				// Traffic-split routers (direct).
				"my-ns-svc-b-ts-8080-traffic-split-direct": {
					Rule:        "Host(`svc-b.my-ns.maesh`) || Host(`10.10.14.1`)",
					EntryPoints: []string{"http-10000"},
					Service:     "my-ns-svc-b-ts-8080-traffic-split",
					Priority:    4001,
					Middlewares: []string{"my-ns-svc-b", "my-ns-svc-b-ts-whitelist-traffic-split-direct"},
				},
			},
			Services: map[string]*dynamic.Service{
				"readiness":         readinessSvc,
				"block-all-service": blockAllService,
				// Traffic-target services.
				"my-ns-svc-b-tt-8080-traffic-target": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						Servers: []dynamic.Server{
							{URL: "http://10.10.2.1:8080"},
							{URL: "http://10.10.2.2:8080"},
						},
						PassHostHeader: getBoolRef(true),
					},
				},
				"my-ns-svc-d-tt-8080-traffic-target": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						Servers: []dynamic.Server{
							{URL: "http://10.10.4.1:8080"},
						},
						PassHostHeader: getBoolRef(true),
					},
				},
				"my-ns-svc-e-tt-8080-traffic-target": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						Servers: []dynamic.Server{
							{URL: "http://10.10.5.1:8080"},
						},
						PassHostHeader: getBoolRef(true),
					},
				},
				// Traffic-split routers.
				"my-ns-svc-b-ts-8080-traffic-split": {
					Weighted: &dynamic.WeightedRoundRobin{
						Services: []dynamic.WRRService{
							{
								Name:   "my-ns-svc-b-ts-8080-svc-d-traffic-split-backend",
								Weight: getIntRef(80),
							},
							{
								Name:   "my-ns-svc-b-ts-8080-svc-e-traffic-split-backend",
								Weight: getIntRef(20),
							},
						},
					},
				},
				// Traffic-split backends routers.
				"my-ns-svc-b-ts-8080-svc-d-traffic-split-backend": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						Servers: []dynamic.Server{
							{URL: "http://svc-d.my-ns.maesh:8080"},
						},
						PassHostHeader: getBoolRef(false),
					},
				},
				"my-ns-svc-b-ts-8080-svc-e-traffic-split-backend": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						Servers: []dynamic.Server{
							{URL: "http://svc-e.my-ns.maesh:8080"},
						},
						PassHostHeader: getBoolRef(false),
					},
				},
			},
			Middlewares: map[string]*dynamic.Middleware{
				"block-all-middleware": blockAllMiddleware,
				// Service B middleware.
				"my-ns-svc-b": {
					Retry: &dynamic.Retry{Attempts: 2},
					RateLimit: &dynamic.RateLimit{
						Average: 100,
						Burst:   110,
					},
					CircuitBreaker: &dynamic.CircuitBreaker{
						Expression: "LatencyAtQuantileMS(50.0) > 100",
					},
				},
				// Traffic-target middleware (direct).
				"my-ns-svc-b-tt-whitelist-traffic-target-direct": {
					IPWhiteList: &dynamic.IPWhiteList{
						SourceRange: []string{"10.10.1.1", "10.10.3.1"},
					},
				},
				"my-ns-svc-d-tt-whitelist-traffic-target-direct": {
					IPWhiteList: &dynamic.IPWhiteList{
						SourceRange: []string{"10.10.1.1", "10.10.3.1"},
					},
				},
				"my-ns-svc-e-tt-whitelist-traffic-target-direct": {
					IPWhiteList: &dynamic.IPWhiteList{
						SourceRange: []string{"10.10.1.1", "10.10.3.1"},
					},
				},
				// Traffic-target middleware (indirect).
				"my-ns-svc-d-tt-whitelist-traffic-target-indirect": {
					IPWhiteList: &dynamic.IPWhiteList{
						SourceRange: []string{
							"10.10.1.1",
							"10.10.3.1",
						},
						IPStrategy: &dynamic.IPStrategy{
							Depth: 1,
						},
					},
				},
				"my-ns-svc-e-tt-whitelist-traffic-target-indirect": {
					IPWhiteList: &dynamic.IPWhiteList{
						SourceRange: []string{
							"10.10.1.1",
							"10.10.3.1",
						},
						IPStrategy: &dynamic.IPStrategy{
							Depth: 1,
						},
					},
				},
				// Traffic-split middleware (direct).
				"my-ns-svc-b-ts-whitelist-traffic-split-direct": {
					IPWhiteList: &dynamic.IPWhiteList{
						SourceRange: []string{
							"10.10.1.1",
							"10.10.3.1",
						},
					},
				},
			},
		},
		TCP: &dynamic.TCPConfiguration{
			Routers:  map[string]*dynamic.TCPRouter{},
			Services: map[string]*dynamic.TCPService{},
		},
	}

	assert.Equal(t, want, got)
}

func nn(name, ns string) topology.NameNamespace {
	return topology.NameNamespace{
		Name:      name,
		Namespace: ns,
	}
}

func svcPort(name string, port, targetPort int32) v1.ServicePort {
	return v1.ServicePort{
		Name:       name,
		Protocol:   "TCP",
		Port:       port,
		TargetPort: intstr.FromInt(int(targetPort)),
	}
}

func createPod(ns, name, sa, ip string) *topology.Pod {
	return &topology.Pod{
		Name:           name,
		Namespace:      ns,
		ServiceAccount: sa,
		IP:             ip,
	}
}

func createSvc(ns, name string, annotations map[string]string, ports []v1.ServicePort, ip string, pods []*topology.Pod) *topology.Service {
	subsetPorts := make([]v1.EndpointPort, len(ports))
	for i, p := range ports {
		subsetPorts[i] = v1.EndpointPort{
			Name:     p.Name,
			Port:     p.TargetPort.IntVal,
			Protocol: "TCP",
		}
	}

	subsetAddress := make([]v1.EndpointAddress, len(pods))
	for i, pod := range pods {
		subsetAddress[i] = v1.EndpointAddress{IP: pod.IP}
	}

	return &topology.Service{
		Name:        name,
		Namespace:   ns,
		Annotations: annotations,
		Ports:       ports,
		ClusterIP:   ip,
		Pods:        pods,
	}
}

func createTrafficSplitBackend(svc *topology.Service, weight int) topology.TrafficSplitBackend {
	return topology.TrafficSplitBackend{
		Weight:  weight,
		Service: svc,
	}
}

func createTrafficSplit(ns, name string, svc *topology.Service, backends []topology.TrafficSplitBackend, incoming []*topology.Pod) *topology.TrafficSplit {
	return &topology.TrafficSplit{
		Name:      name,
		Namespace: ns,
		Service:   svc,
		Backends:  backends,
		Incoming:  incoming,
	}
}

func createHTTPMatch(name string, methods []string, pathRegex string) spec.HTTPMatch {
	return spec.HTTPMatch{
		Name:      name,
		Methods:   methods,
		PathRegex: pathRegex,
	}
}

func createHTTPRouteGroup(ns, name string, matches []spec.HTTPMatch) *spec.HTTPRouteGroup {
	return &spec.HTTPRouteGroup{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRouteGroup",
			APIVersion: "specs.smi-spec.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Matches: matches,
	}
}

func createTrafficTargetSource(ns, sa string, pods []*topology.Pod) topology.ServiceTrafficTargetSource {
	return topology.ServiceTrafficTargetSource{
		ServiceAccount: sa,
		Namespace:      ns,
		Pods:           pods,
	}
}

func createTrafficTargetDestination(ns, sa string, pods []*topology.Pod, ports []v1.ServicePort) topology.ServiceTrafficTargetDestination {
	return topology.ServiceTrafficTargetDestination{
		ServiceAccount: sa,
		Namespace:      ns,
		Ports:          ports,
		Pods:           pods,
	}
}

func createTrafficTarget(name string, svc *topology.Service, sources []topology.ServiceTrafficTargetSource, dest topology.ServiceTrafficTargetDestination, specs []topology.TrafficSpec) *topology.ServiceTrafficTarget {
	return &topology.ServiceTrafficTarget{
		Service:     svc,
		Name:        name,
		Sources:     sources,
		Destination: dest,
		Specs:       specs,
	}
}

func createTCPRoute(ns, name string) *spec.TCPRoute {
	return &spec.TCPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TCPRoute",
			APIVersion: "specs.smi-spec.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

func getBoolRef(v bool) *bool {
	return &v
}

func getIntRef(v int) *int {
	return &v
}

var readinessRtr = &dynamic.Router{
	Rule:        "Path(`/ping`)",
	EntryPoints: []string{"readiness"},
	Service:     "readiness",
}

var readinessSvc = &dynamic.Service{
	LoadBalancer: &dynamic.ServersLoadBalancer{
		PassHostHeader: getBoolRef(true),
		Servers: []dynamic.Server{
			{
				URL: "http://127.0.0.1:8080",
			},
		},
	},
}

var blockAllMiddleware = &dynamic.Middleware{
	IPWhiteList: &dynamic.IPWhiteList{
		SourceRange: []string{"255.255.255.255"},
	},
}

var blockAllService = &dynamic.Service{
	LoadBalancer: &dynamic.ServersLoadBalancer{},
}
