package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/mesh/v2/pkg/annotations"
	"github.com/traefik/mesh/v2/pkg/k8s"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	listers "k8s.io/client-go/listers/core/v1"
)

const (
	testNamespace          = "test"
	testDefaultTrafficType = annotations.ServiceTypeHTTP
)

func TestShadowServiceManager_LoadPortMapping(t *testing.T) {
	logger := logrus.New()

	svc1 := newFakeService("svc-1", map[int]int{8000: 80}, annotations.ServiceTypeTCP)
	svc2 := newFakeService("svc-2", map[int]int{8000: 80}, annotations.ServiceTypeTCP)

	shadowSvc1 := newFakeShadowService(t, svc1, map[int]int{8000: 5000})
	shadowSvc2 := newFakeShadowService(t, svc2, map[int]int{8000: 5001})

	// Add an incompatible port: UDP in a TCP.
	shadowSvc1.Spec.Ports = append(shadowSvc1.Spec.Ports, corev1.ServicePort{
		Name:       "incompatible-port",
		Protocol:   corev1.ProtocolUDP,
		Port:       9000,
		TargetPort: intstr.FromInt(5002),
	})

	tcpPortMapper := &portMappingMock{
		t: t,
		setCalledWith: []portMapping{
			{namespace: svc1.Namespace, name: svc1.Name, fromPort: 8000, toPort: 5000},
			{namespace: svc2.Namespace, name: svc2.Name, fromPort: 8000, toPort: 5001},
		},
	}

	_, svcLister := newFakeK8sClient(t, svc1, svc2, shadowSvc1, shadowSvc2)

	mgr := ShadowServiceManager{
		namespace:          testNamespace,
		defaultTrafficType: testDefaultTrafficType,
		serviceLister:      svcLister,
		tcpStateTable:      tcpPortMapper,
		logger:             logger,
	}

	assert.NoError(t, mgr.LoadPortMapping())

	assert.Equal(t, 2, tcpPortMapper.setCounter)
}

// TestShadowServiceManager_SyncServiceHandlesUnknownTrafficTypes tests the case where a service is updated with an
// invalid traffic type. It makes sure the shadow service won't be updated.
func TestShadowServiceManager_SyncServiceHandlesUnknownTrafficTypes(t *testing.T) {
	logger := logrus.New()

	// Create a service and simulate an update on the ports from 8000 to 9000 and with an invalid traffic type.
	svc := newFakeService("svc", map[int]int{9000: 80}, "pigeon")

	shadowSvc := newFakeShadowService(t, svc, map[int]int{8000: 5000})

	client, svcLister := newFakeK8sClient(t, svc, shadowSvc)

	mgr := ShadowServiceManager{
		namespace:          testNamespace,
		defaultTrafficType: testDefaultTrafficType,
		kubeClient:         client,
		serviceLister:      svcLister,
		logger:             logger,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.Error(t, mgr.SyncService(ctx, svc.Namespace, svc.Name))

	// Make sure the shadow service stays intact.
	syncedShadowSvc, err := client.CoreV1().Services(testNamespace).Get(ctx, shadowSvc.Name, metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, shadowSvc.Annotations, syncedShadowSvc.Annotations)
	assert.Equal(t, shadowSvc.Spec.Ports, syncedShadowSvc.Spec.Ports)
}

// TestShadowServiceManager_SyncServiceHandlesServiceLabelsMismatch tests the case where the service labels in the shadow service
// does not match the synced service due to a hash collision.
func TestShadowServiceManager_SyncServiceHandlesServiceLabelsMismatch(t *testing.T) {
	logger := logrus.New()

	// Create a service and simulate an update on the ports from 8000 to 9000 and with an invalid traffic type.
	svc := newFakeService("svc", map[int]int{9000: 80}, "pigeon")

	shadowSvc := newFakeShadowService(t, svc, map[int]int{8000: 5000})

	// Modify the service labels in the shadow service to simulate a hash collision.
	shadowSvc.Labels[k8s.LabelServiceName] = "name"
	shadowSvc.Labels[k8s.LabelServiceNamespace] = "namespace"

	client, svcLister := newFakeK8sClient(t, svc, shadowSvc)

	mgr := ShadowServiceManager{
		namespace:          testNamespace,
		defaultTrafficType: testDefaultTrafficType,
		kubeClient:         client,
		serviceLister:      svcLister,
		logger:             logger,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.Error(t, mgr.SyncService(ctx, svc.Namespace, svc.Name))

	// Make sure the shadow service stays intact.
	syncedShadowSvc, err := client.CoreV1().Services(testNamespace).Get(ctx, shadowSvc.Name, metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, shadowSvc.Annotations, syncedShadowSvc.Annotations)
	assert.Equal(t, shadowSvc.Spec.Ports, syncedShadowSvc.Spec.Ports)
}

// TestShadowServiceManager_SyncServiceCreateShadowService tests the case where a service has been created. It makes
// sure the shadow service is created.
func TestShadowServiceManager_SyncServiceCreateShadowService(t *testing.T) {
	logger := logrus.New()

	svc := newFakeService("svc", map[int]int{9000: 8080, 9001: 8081}, annotations.ServiceTypeHTTP)

	httpPortMapper := &portMappingMock{
		t: t,
		addCalledWith: []portMapping{
			{namespace: svc.Namespace, name: svc.Name, fromPort: 9000, toPort: 5000},
			{namespace: svc.Namespace, name: svc.Name, fromPort: 9001, toPort: 5001},
		},
	}

	client, svcLister := newFakeK8sClient(t, svc)

	mgr := ShadowServiceManager{
		namespace:          testNamespace,
		defaultTrafficType: testDefaultTrafficType,
		kubeClient:         client,
		serviceLister:      svcLister,
		httpStateTable:     httpPortMapper,
		logger:             logger,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	assert.NoError(t, mgr.SyncService(ctx, svc.Namespace, svc.Name))

	// Make sure the shadow service has been created.
	shadowSvcName, err := GetShadowServiceName(svc.Namespace, svc.Name)
	require.NoError(t, err)

	shadowSvc, err := client.CoreV1().Services(testNamespace).Get(ctx, shadowSvcName, metav1.GetOptions{})
	require.NoError(t, err)

	assert.Len(t, shadowSvc.Spec.Ports, 2)
	assert.ElementsMatch(t, shadowSvc.Spec.Ports, []corev1.ServicePort{
		{
			Name:       "port-9000",
			Protocol:   corev1.ProtocolTCP,
			Port:       9000,
			TargetPort: intstr.FromInt(5000),
		},
		{
			Name:       "port-9001",
			Protocol:   corev1.ProtocolTCP,
			Port:       9001,
			TargetPort: intstr.FromInt(5001),
		},
	})

	assert.Equal(t, 2, httpPortMapper.addCounter)
}

// TestShadowServiceManager_SyncServiceUpdateShadowService tests the case where a service has been updated and
// the shadow service already exist. It makes sure the shadow service is updated accordingly.
func TestShadowServiceManager_SyncServiceUpdateShadowService(t *testing.T) {
	logger := logrus.New()

	// Create a service and simulate an update on the ports.
	svc := newFakeService("svc", map[int]int{8000: 80, 9001: 8081}, annotations.ServiceTypeHTTP)
	updatedSvc := newFakeService("svc", map[int]int{9000: 8080, 9001: 8081}, annotations.ServiceTypeHTTP)

	shadowSvc := newFakeShadowService(t, svc, map[int]int{8000: 5000, 9001: 5001})

	httpPortMapper := &portMappingMock{
		t: t,
		removeCalledWith: []portMapping{
			{namespace: svc.Namespace, name: svc.Name, fromPort: 8000, toPort: 5000},
		},
		addCalledWith: []portMapping{
			{namespace: svc.Namespace, name: svc.Name, fromPort: 9000, toPort: 5000},
			{namespace: svc.Namespace, name: svc.Name, fromPort: 9001, toPort: 5001},
		},
	}

	client, svcLister := newFakeK8sClient(t, updatedSvc, shadowSvc)

	mgr := ShadowServiceManager{
		namespace:          testNamespace,
		defaultTrafficType: testDefaultTrafficType,
		kubeClient:         client,
		serviceLister:      svcLister,
		httpStateTable:     httpPortMapper,
		logger:             logger,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	assert.NoError(t, mgr.SyncService(ctx, svc.Namespace, svc.Name))

	// Make sure the shadow service has been updated.
	updateShadowSvc, err := client.CoreV1().Services(testNamespace).Get(ctx, shadowSvc.Name, metav1.GetOptions{})
	require.NoError(t, err)

	assert.Len(t, updateShadowSvc.Spec.Ports, 2)
	assert.Equal(t, corev1.ProtocolTCP, updateShadowSvc.Spec.Ports[0].Protocol)

	assert.ElementsMatch(t, updateShadowSvc.Spec.Ports, []corev1.ServicePort{
		{
			Name:       "port-9000",
			Protocol:   corev1.ProtocolTCP,
			Port:       9000,
			TargetPort: intstr.FromInt(5000),
		},
		{
			Name:       "port-9001",
			Protocol:   corev1.ProtocolTCP,
			Port:       9001,
			TargetPort: intstr.FromInt(5001),
		},
	})

	assert.Equal(t, 1, httpPortMapper.removeCounter)
	assert.Equal(t, 2, httpPortMapper.addCounter)
}

// TestShadowServiceManager_SyncServiceUpdateShadowServicesAndHandleTrafficTypeChanges tests the case a service has
// been updated and its traffic type has changed.
func TestShadowServiceManager_SyncServiceUpdateShadowServicesAndHandleTrafficTypeChanges(t *testing.T) {
	logger := logrus.New()

	// Create a service and simulate an update on the ports and traffic type.
	svc := newFakeService("svc", map[int]int{8000: 80}, annotations.ServiceTypeHTTP)
	updatedSvc := newFakeService("svc", map[int]int{9000: 1010}, annotations.ServiceTypeUDP)

	shadowSvc := newFakeShadowService(t, svc, map[int]int{8000: 5000})

	httpPortMapper := &portMappingMock{
		t: t,
		removeCalledWith: []portMapping{
			{namespace: svc.Namespace, name: svc.Name, fromPort: 8000, toPort: 5000},
		},
	}
	udpPortMapper := &portMappingMock{
		t: t,
		addCalledWith: []portMapping{
			{namespace: svc.Namespace, name: svc.Name, fromPort: 9000, toPort: 10000},
		},
	}

	client, svcLister := newFakeK8sClient(t, updatedSvc, shadowSvc)

	mgr := ShadowServiceManager{
		namespace:          testNamespace,
		defaultTrafficType: testDefaultTrafficType,
		kubeClient:         client,
		serviceLister:      svcLister,
		httpStateTable:     httpPortMapper,
		udpStateTable:      udpPortMapper,
		logger:             logger,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	assert.NoError(t, mgr.SyncService(ctx, svc.Namespace, svc.Name))

	// Make sure the shadow service has been updated.
	updateShadowSvc, err := client.CoreV1().Services(testNamespace).Get(ctx, shadowSvc.Name, metav1.GetOptions{})
	require.NoError(t, err)

	trafficType, err := annotations.GetTrafficType(updateShadowSvc.Annotations)
	require.NoError(t, err)
	assert.Equal(t, annotations.ServiceTypeUDP, trafficType)

	assert.Len(t, updateShadowSvc.Spec.Ports, 1)
	assert.Equal(t, corev1.ProtocolUDP, updateShadowSvc.Spec.Ports[0].Protocol)
	assert.Equal(t, int32(9000), updateShadowSvc.Spec.Ports[0].Port)
	assert.Equal(t, int32(10000), updateShadowSvc.Spec.Ports[0].TargetPort.IntVal)

	assert.Equal(t, 1, httpPortMapper.removeCounter)
	assert.Equal(t, 1, udpPortMapper.addCounter)
}

// TestShadowServiceManager_SyncServiceDeleteShadowServices checks the case where the given service has been removed
// and there are still some shadow services left.
func TestShadowServiceManager_SyncServiceDeleteShadowServices(t *testing.T) {
	logger := logrus.New()

	// Simulate a service that have been removed.
	svc := newFakeService("svc", map[int]int{8000: 80}, annotations.ServiceTypeHTTP)

	shadowSvc := newFakeShadowService(t, svc, map[int]int{8000: 5000})

	httpPortMapper := &portMappingMock{
		t: t,
		removeCalledWith: []portMapping{
			{namespace: svc.Namespace, name: svc.Name, fromPort: 8000, toPort: 5000},
		},
	}

	client, svcLister := newFakeK8sClient(t, shadowSvc)

	mgr := ShadowServiceManager{
		namespace:          testNamespace,
		defaultTrafficType: testDefaultTrafficType,
		kubeClient:         client,
		serviceLister:      svcLister,
		httpStateTable:     httpPortMapper,
		logger:             logger,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	assert.NoError(t, mgr.SyncService(ctx, svc.Namespace, svc.Name))

	// Check if the shadow service have been removed.
	_, err := client.CoreV1().Services(testNamespace).Get(ctx, shadowSvc.Name, metav1.GetOptions{})
	assert.True(t, kerrors.IsNotFound(err))

	assert.Equal(t, 1, httpPortMapper.removeCounter)
}

func newFakeService(name string, ports map[int]int, trafficType string) *corev1.Service {
	var svcPorts []corev1.ServicePort

	protocol := corev1.ProtocolTCP
	if trafficType == annotations.ServiceTypeUDP {
		protocol = corev1.ProtocolUDP
	}

	for port, targetPort := range ports {
		svcPorts = append(svcPorts, corev1.ServicePort{
			Name:       fmt.Sprintf("port-%d", port),
			Protocol:   protocol,
			Port:       int32(port),
			TargetPort: intstr.FromInt(targetPort),
		})
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "default",
			Annotations: map[string]string{},
		},
		Spec: corev1.ServiceSpec{
			Ports: svcPorts,
		},
	}

	if trafficType != "" {
		annotations.SetTrafficType(trafficType, svc.Annotations)
	}

	return svc
}

func newFakeShadowService(t *testing.T, svc *corev1.Service, ports map[int]int) *corev1.Service {
	t.Helper()

	var svcPorts []corev1.ServicePort

	name, err := GetShadowServiceName(svc.Namespace, svc.Name)
	require.NoError(t, err)

	trafficType, err := annotations.GetTrafficType(svc.Annotations)
	if errors.Is(err, annotations.ErrNotFound) {
		trafficType = testDefaultTrafficType
	}

	protocol := corev1.ProtocolTCP
	if trafficType == annotations.ServiceTypeUDP {
		protocol = corev1.ProtocolUDP
	}

	for port, targetPort := range ports {
		svcPorts = append(svcPorts, corev1.ServicePort{
			Name:       fmt.Sprintf("port-%d", port),
			Protocol:   protocol,
			Port:       int32(port),
			TargetPort: intstr.FromInt(targetPort),
		})
	}

	labels := k8s.ShadowServiceLabels()
	labels[k8s.LabelServiceNamespace] = svc.Namespace
	labels[k8s.LabelServiceName] = svc.Name

	shadowSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   testNamespace,
			Labels:      labels,
			Annotations: map[string]string{},
		},
		Spec: corev1.ServiceSpec{
			Selector: k8s.ProxyLabels(),
			Ports:    svcPorts,
		},
	}

	if trafficType != "" {
		annotations.SetTrafficType(trafficType, shadowSvc.Annotations)
	}

	return shadowSvc
}

func newFakeK8sClient(t *testing.T, objects ...runtime.Object) (*fake.Clientset, listers.ServiceLister) {
	t.Helper()

	client := fake.NewSimpleClientset(objects...)

	informerFactory := informers.NewSharedInformerFactory(client, 5*time.Minute)
	svcLister := informerFactory.Core().V1().Services().Lister()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	informerFactory.Start(ctx.Done())

	for typ, ok := range informerFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			require.NoError(t, fmt.Errorf("timed out waiting for controller caches to sync: %s", typ))
		}
	}

	return client, svcLister
}

func TestShadowServiceManager_isPortCompatible(t *testing.T) {
	tests := []struct {
		desc           string
		trafficType    string
		portProtocol   corev1.Protocol
		expectedResult bool
	}{
		{
			desc:           "should return true if the traffic type is udp and the port protocol is UDP",
			trafficType:    annotations.ServiceTypeUDP,
			portProtocol:   corev1.ProtocolUDP,
			expectedResult: true,
		},
		{
			desc:           "should return false if the traffic type is udp and the port protocol is not UDP",
			trafficType:    annotations.ServiceTypeUDP,
			portProtocol:   corev1.ProtocolSCTP,
			expectedResult: false,
		},
		{
			desc:           "should return true if the traffic type is http and the port protocol is TCP",
			trafficType:    annotations.ServiceTypeHTTP,
			portProtocol:   corev1.ProtocolTCP,
			expectedResult: true,
		},
		{
			desc:           "should return true if the traffic type is tcp and the port protocol is TCP",
			trafficType:    annotations.ServiceTypeTCP,
			portProtocol:   corev1.ProtocolTCP,
			expectedResult: true,
		},
		{
			desc:           "should return false if the traffic type is http and the port protocol is not TCP",
			trafficType:    annotations.ServiceTypeHTTP,
			portProtocol:   corev1.ProtocolSCTP,
			expectedResult: false,
		},
		{
			desc:           "should return false if the traffic type is http and the port protocol is not TCP",
			trafficType:    annotations.ServiceTypeTCP,
			portProtocol:   corev1.ProtocolUDP,
			expectedResult: false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			result := isPortCompatible(test.trafficType, corev1.ServicePort{
				Protocol: test.portProtocol,
			})

			assert.Equal(t, test.expectedResult, result)
		})
	}
}

type portMapping struct {
	namespace string
	name      string
	fromPort  int32
	toPort    int32
	called    bool
}

type portMappingMock struct {
	t *testing.T

	setCalledWith    []portMapping
	addCalledWith    []portMapping
	findCalledWith   []portMapping
	removeCalledWith []portMapping

	setCounter    int
	addCounter    int
	findCounter   int
	removeCounter int
}

func (m *portMappingMock) Set(namespace, name string, fromPort, toPort int32) error {
	if m.setCalledWith == nil {
		assert.FailNowf(m.t, "Set has been called", "%s/%s %d-%d", name, namespace, fromPort, toPort)
	}

	m.setCounter++

	for _, callArgs := range m.setCalledWith {
		if callArgs.called {
			continue
		}

		if namespace == callArgs.namespace && name == callArgs.name && fromPort == callArgs.fromPort && toPort == callArgs.toPort {
			callArgs.called = true

			return nil
		}
	}

	assert.FailNowf(m.t, "unexpected call to Set", "%s/%s %d->%d", name, namespace, fromPort, toPort)

	return errors.New("fail")
}

func (m *portMappingMock) Add(namespace, name string, fromPort int32) (int32, error) {
	if m.addCalledWith == nil {
		assert.FailNowf(m.t, "Add has been called", "%s/%s %d", name, namespace, fromPort)
	}

	m.addCounter++

	for _, callArgs := range m.addCalledWith {
		if callArgs.called {
			continue
		}

		if namespace == callArgs.namespace && name == callArgs.name && fromPort == callArgs.fromPort {
			callArgs.called = true

			return callArgs.toPort, nil
		}
	}

	assert.FailNowf(m.t, "unexpected call to Add", "%s/%s %d", name, namespace, fromPort)

	return 0, errors.New("fail")
}

func (m *portMappingMock) Find(namespace, name string, fromPort int32) (int32, bool) {
	if m.findCalledWith == nil {
		assert.FailNowf(m.t, "Find has been called", "%s/%s %d", name, namespace, fromPort)
	}

	m.findCounter++

	for _, callArgs := range m.findCalledWith {
		if callArgs.called {
			continue
		}

		if namespace == callArgs.namespace && name == callArgs.name && fromPort == callArgs.fromPort {
			callArgs.called = true

			return callArgs.toPort, true
		}
	}

	assert.FailNowf(m.t, "unexpected call to Find", "%s/%s %d", name, namespace, fromPort)

	return 0, false
}

func (m *portMappingMock) Remove(namespace, name string, fromPort int32) (int32, bool) {
	if m.removeCalledWith == nil {
		assert.FailNowf(m.t, "Remove has been called", "%s/%s %d", name, namespace, fromPort)
	}

	m.removeCounter++

	for _, callArgs := range m.removeCalledWith {
		if callArgs.called {
			continue
		}

		if namespace == callArgs.namespace && name == callArgs.name && fromPort == callArgs.fromPort {
			callArgs.called = true

			return callArgs.toPort, true
		}
	}

	assert.FailNowf(m.t, "unexpected call to Remove", "%s/%s %d", name, namespace, fromPort)

	return 0, false
}
