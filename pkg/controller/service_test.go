package controller

import (
	"context"
	"os"
	"testing"
	"time"

	goversion "github.com/hashicorp/go-version"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	listers "k8s.io/client-go/listers/core/v1"
)

type portMapperMock struct {
	findFunc   func(namespace, name string, port int32) (int32, bool)
	addFunc    func(namespace, name string, port int32) (int32, error)
	removeFunc func(namespace, name string, port int32) (int32, error)
}

func (t portMapperMock) Find(namespace, name string, port int32) (int32, bool) {
	if t.findFunc == nil {
		return 0, false
	}

	return t.findFunc(namespace, name, port)
}

func (t portMapperMock) Add(namespace, name string, port int32) (int32, error) {
	if t.addFunc == nil {
		return 0, nil
	}

	return t.addFunc(namespace, name, port)
}

func (t portMapperMock) Remove(namespace, name string, port int32) (int32, error) {
	if t.removeFunc == nil {
		return 0, nil
	}

	return t.removeFunc(namespace, name, port)
}

func TestShadowServiceManager_CreateOrUpdate(t *testing.T) {
	tests := []struct {
		desc              string
		defaultMode       string
		svc               *corev1.Service
		serverVersion     string
		currentShadowSvc  *corev1.Service
		expectedShadowSvc *corev1.Service
	}{
		{
			desc:        "should create a shadow service",
			defaultMode: "tcp",
			svc: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Protocol: corev1.ProtocolTCP,
							Port:     8080,
						},
					},
				},
			},
			expectedShadowSvc: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-foo-6d61657368-bar",
					Namespace: "maesh",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Protocol:   corev1.ProtocolTCP,
							Port:       8080,
							TargetPort: intstr.FromInt(10000),
						},
					},
				},
			},
		},
		{
			desc:          "should create a shadow service without the topology keys",
			defaultMode:   "tcp",
			serverVersion: "v1.16.6-beta.0",
			svc: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Protocol: corev1.ProtocolTCP,
							Port:     8080,
						},
					},
				},
			},
			expectedShadowSvc: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-foo-6d61657368-bar",
					Namespace: "maesh",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Protocol:   corev1.ProtocolTCP,
							Port:       8080,
							TargetPort: intstr.FromInt(10000),
						},
					},
				},
			},
		},
		{
			desc:        "should update the existing shadow service and remove the unused port",
			defaultMode: "tcp",
			svc: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Protocol: corev1.ProtocolTCP, Port: 8080},
					},
				},
			},
			currentShadowSvc: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-foo-6d61657368-bar",
					Namespace: "maesh",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Protocol:   corev1.ProtocolTCP,
							Port:       8080,
							TargetPort: intstr.FromInt(10005),
						},
						{
							Protocol:   corev1.ProtocolTCP,
							Port:       8081,
							TargetPort: intstr.FromInt(10001),
						},
					},
				},
			},
			expectedShadowSvc: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-foo-6d61657368-bar",
					Namespace: "maesh",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Protocol:   corev1.ProtocolTCP,
							Port:       8080,
							TargetPort: intstr.FromInt(10000),
						},
					},
				},
			},
		},
		{
			desc:        "should update existing shadow service and reuse port mappings",
			defaultMode: "tcp",
			svc: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Protocol: corev1.ProtocolTCP, Port: 8080},
					},
				},
			},
			currentShadowSvc: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-foo-6d61657368-bar",
					Namespace: "maesh",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Protocol:   corev1.ProtocolTCP,
							Port:       8080,
							TargetPort: intstr.FromInt(10000),
						},
					},
				},
			},
			expectedShadowSvc: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-foo-6d61657368-bar",
					Namespace: "maesh",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Protocol:   corev1.ProtocolTCP,
							Port:       8080,
							TargetPort: intstr.FromInt(10000),
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			currentShadowServices := make([]runtime.Object, 0)
			if test.currentShadowSvc != nil {
				currentShadowServices = append(currentShadowServices, test.currentShadowSvc)
			}

			serverVersionStr := "v1.17"
			if test.serverVersion != "" {
				serverVersionStr = test.serverVersion
			}

			client, lister := newFakeClient(serverVersionStr, currentShadowServices...)

			tcpPortMapperMock := portMapperMock{
				findFunc: func(namespace, name string, port int32) (int32, bool) {
					return 0, false
				},
				addFunc: func(namespace, name string, port int32) (int32, error) {
					return 10000, nil
				},
				removeFunc: func(namespace, name string, port int32) (int32, error) {
					return 10000, nil
				},
			}

			shadowServiceManager := NewShadowServiceManager(
				log,
				lister,
				"maesh",
				tcpPortMapperMock,
				portMapperMock{},
				test.defaultMode,
				5000,
				5002,
				client,
			)

			shadowSvc, err := shadowServiceManager.CreateOrUpdate(test.svc)
			require.NoError(t, err)

			assert.Equal(t, test.expectedShadowSvc.Name, shadowSvc.Name)
			assert.Equal(t, test.expectedShadowSvc.Namespace, shadowSvc.Namespace)

			for i, port := range test.expectedShadowSvc.Spec.Ports {
				assert.Equal(t, port, shadowSvc.Spec.Ports[i])
			}

			serverVersion, err := goversion.NewVersion(serverVersionStr)
			require.NoError(t, err)

			if serverVersion.GreaterThanOrEqual(versionTopologyKeys) {
				assert.True(t, len(shadowSvc.Spec.TopologyKeys) > 0)
			}
		})
	}
}

func TestShadowServiceManager_Delete(t *testing.T) {
	tests := []struct {
		desc             string
		name             string
		namespace        string
		currentShadowSvc *corev1.Service
	}{
		{
			desc:      "should return nil if the corresponding shadow service cannot be found",
			name:      "foo",
			namespace: "bar",
		},
		{
			desc:      "should remove the TCP ports mapped for the deleted service",
			name:      "foo",
			namespace: "bar",
			currentShadowSvc: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-foo-6d61657368-bar",
					Namespace: "maesh",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10000),
							Port:       8080,
						},
					},
				},
			},
		},
		{
			desc:      "should remove the UDP ports mapped for the deleted service",
			name:      "foo",
			namespace: "bar",
			currentShadowSvc: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-foo-6d61657368-bar",
					Namespace: "maesh",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Protocol:   corev1.ProtocolUDP,
							TargetPort: intstr.FromInt(15000),
							Port:       8081,
						},
					},
				},
			},
		},
		{
			desc:      "should remove the UDP and TCP ports mapped for the deleted service",
			name:      "foo",
			namespace: "bar",
			currentShadowSvc: &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-foo-6d61657368-bar",
					Namespace: "maesh",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10000),
							Port:       8080,
						},
						{
							Protocol:   corev1.ProtocolUDP,
							TargetPort: intstr.FromInt(15000),
							Port:       8081,
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			removedUDPPorts := make(map[servicePort]bool)
			udpPortMapperMock := portMapperMock{
				removeFunc: func(namespace, name string, port int32) (int32, error) {
					removedUDPPorts[servicePort{Namespace: namespace, Name: name, Port: port}] = true
					return 0, nil
				},
			}

			removedTCPPorts := make(map[servicePort]bool)
			tcpPortMapperMock := portMapperMock{
				removeFunc: func(namespace, name string, port int32) (int32, error) {
					removedTCPPorts[servicePort{Namespace: namespace, Name: name, Port: port}] = true
					return 0, nil
				},
			}

			currentShadowServices := make([]runtime.Object, 0)
			if test.currentShadowSvc != nil {
				currentShadowServices = append(currentShadowServices, test.currentShadowSvc)
			}

			client, lister := newFakeClient("v1.17", currentShadowServices...)

			shadowServiceManager := NewShadowServiceManager(
				log,
				lister,
				"maesh",
				tcpPortMapperMock,
				udpPortMapperMock,
				"http",
				5000,
				5002,
				client,
			)

			err := shadowServiceManager.Delete(test.namespace, test.name)
			require.NoError(t, err)

			if test.currentShadowSvc == nil {
				assert.Equal(t, 0, len(removedTCPPorts))
				assert.Equal(t, 0, len(removedUDPPorts))
				return
			}

			for _, svcPort := range test.currentShadowSvc.Spec.Ports {
				svcWithPort := servicePort{
					Namespace: test.namespace,
					Name:      test.name,
					Port:      svcPort.Port,
				}

				switch svcPort.Protocol {
				case corev1.ProtocolTCP:
					assert.True(t, removedTCPPorts[svcWithPort])

				case corev1.ProtocolUDP:
					assert.True(t, removedUDPPorts[svcWithPort])

				default:
					t.Fail()
				}
			}
		})
	}
}

func TestShadowServiceManager_getShadowServiceName(t *testing.T) {
	name := "foo"
	namespace := "bar"

	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	client, lister := newFakeClient("v1.17")

	shadowServiceManager := NewShadowServiceManager(
		log,
		lister,
		"maesh",
		portMapperMock{},
		portMapperMock{},
		"http",
		5000,
		5002,
		client,
	)

	shadowSvcName := shadowServiceManager.getShadowServiceName(namespace, name)

	assert.Equal(t, shadowSvcName, "maesh-foo-6d61657368-bar")
}

func TestShadowServiceManager_getHTTPPort(t *testing.T) {
	tests := []struct {
		desc        string
		portID      int
		expectedErr bool
	}{
		{
			desc:        "should return an error if no HTTP port mapping is available",
			portID:      3,
			expectedErr: true,
		},
		{
			desc:        "should return the HTTP port mapping associated with the given portID",
			portID:      0,
			expectedErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			client, lister := newFakeClient("v1.17")

			minHTTPPort := int32(5000)
			maxHTTPPort := int32(5002)

			shadowServiceManager := NewShadowServiceManager(
				log,
				lister,
				"maesh",
				portMapperMock{},
				portMapperMock{},
				"http",
				minHTTPPort,
				maxHTTPPort,
				client,
			)

			port, err := shadowServiceManager.getHTTPPort(test.portID)
			if test.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, minHTTPPort+int32(test.portID), port)
		})
	}
}

func TestShadowServiceManager_isPortSuitable(t *testing.T) {
	tests := []struct {
		desc           string
		trafficType    string
		portProtocol   corev1.Protocol
		expectedResult bool
	}{
		{
			desc:           "should return true if the traffic type is udp and the port protocol is UDP",
			trafficType:    "udp",
			portProtocol:   corev1.ProtocolUDP,
			expectedResult: true,
		},
		{
			desc:           "should return false if the traffic type is udp and the port protocol is not UDP",
			trafficType:    "udp",
			portProtocol:   corev1.ProtocolSCTP,
			expectedResult: false,
		},
		{
			desc:           "should return true if the traffic type is http and the port protocol is TCP",
			trafficType:    "http",
			portProtocol:   corev1.ProtocolTCP,
			expectedResult: true,
		},
		{
			desc:           "should return true if the traffic type is tcp and the port protocol is TCP",
			trafficType:    "tcp",
			portProtocol:   corev1.ProtocolTCP,
			expectedResult: true,
		},
		{
			desc:           "should return false if the traffic type is http and the port protocol is not TCP",
			trafficType:    "http",
			portProtocol:   corev1.ProtocolSCTP,
			expectedResult: false,
		},
		{
			desc:           "should return false if the traffic type is http and the port protocol is not TCP",
			trafficType:    "tcp",
			portProtocol:   corev1.ProtocolUDP,
			expectedResult: false,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			result := isPortSuitable(test.trafficType, corev1.ServicePort{
				Protocol: test.portProtocol,
			})

			assert.Equal(t, test.expectedResult, result)
		})
	}
}

func TestShadowServiceManager_containsPort(t *testing.T) {
	tests := []struct {
		desc           string
		port           corev1.ServicePort
		ports          []corev1.ServicePort
		expectedResult bool
	}{
		{
			desc: "should return true if the given service port exists",
			port: corev1.ServicePort{Port: 80, Protocol: corev1.ProtocolTCP},
			ports: []corev1.ServicePort{
				{Port: 80, Protocol: corev1.ProtocolTCP},
			},
			expectedResult: true,
		},
		{
			desc:           "should return false if the given service port list is empty",
			port:           corev1.ServicePort{Port: 8080, Protocol: corev1.ProtocolTCP},
			expectedResult: false,
		},
		{
			desc: "should return false if the given service port does not have the same port",
			port: corev1.ServicePort{Port: 8080, Protocol: corev1.ProtocolTCP},
			ports: []corev1.ServicePort{
				{Port: 80, Protocol: corev1.ProtocolTCP},
			},
			expectedResult: false,
		},
		{
			desc: "should return false if the given service port does not have the same protocol",
			port: corev1.ServicePort{Port: 80, Protocol: corev1.ProtocolUDP},
			ports: []corev1.ServicePort{
				{Port: 80, Protocol: corev1.ProtocolTCP},
			},
			expectedResult: false,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			result := containsPort(test.ports, test.port)

			assert.Equal(t, test.expectedResult, result)
		})
	}
}

func TestShadowServiceManager_needsCleanup(t *testing.T) {
	tests := []struct {
		desc           string
		port           corev1.ServicePort
		ports          []corev1.ServicePort
		expectedResult bool
	}{
		{
			desc: "should return false if the given service port exists",
			port: corev1.ServicePort{Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(80)},
			ports: []corev1.ServicePort{
				{Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(80)},
			},
			expectedResult: false,
		},
		{
			desc:           "should return true if the given service port list is empty",
			port:           corev1.ServicePort{Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(80)},
			expectedResult: true,
		},
		{
			desc: "should return true if the given service port does not have the same port",
			port: corev1.ServicePort{Port: 90, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(80)},
			ports: []corev1.ServicePort{
				{Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(80)},
			},
			expectedResult: true,
		},
		{
			desc: "should return true if the given service port does not have the same protocol",
			port: corev1.ServicePort{Port: 80, Protocol: corev1.ProtocolUDP, TargetPort: intstr.FromInt(80)},
			ports: []corev1.ServicePort{
				{Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(80)},
			},
			expectedResult: true,
		},
		{
			desc: "should return true if the given service port does not have the same target port",
			port: corev1.ServicePort{Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(90)},
			ports: []corev1.ServicePort{
				{Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(80)},
			},
			expectedResult: true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			result := needsCleanup(test.ports, test.port)

			assert.Equal(t, test.expectedResult, result)
		})
	}
}

func newFakeClient(serverVersion string, objects ...runtime.Object) (*fake.Clientset, listers.ServiceLister) {
	client := fake.NewSimpleClientset(objects...)

	discovery, _ := client.Discovery().(*fakediscovery.FakeDiscovery)
	discovery.FakedServerVersion = &version.Info{
		GitVersion: serverVersion,
	}

	informerFactory := informers.NewSharedInformerFactory(client, 5*time.Minute)
	lister := informerFactory.Core().V1().Services().Lister()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stop := make(<-chan struct{})
	informerFactory.Start(stop)
	informerFactory.WaitForCacheSync(ctx.Done())

	return client, lister
}
