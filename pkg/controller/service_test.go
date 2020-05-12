package controller_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/containous/maesh/pkg/controller"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	listers "k8s.io/client-go/listers/core/v1"
)

type portMapperMock struct {
	findFunc   func(svc k8s.ServiceWithPort) (int32, bool)
	addFunc    func(svc *k8s.ServiceWithPort) (int32, error)
	removeFunc func(svc k8s.ServiceWithPort) (int32, error)
}

func (t portMapperMock) Find(svc k8s.ServiceWithPort) (int32, bool) {
	return t.findFunc(svc)
}

func (t portMapperMock) Add(svc *k8s.ServiceWithPort) (int32, error) {
	return t.addFunc(svc)
}

func (t portMapperMock) Remove(svc k8s.ServiceWithPort) (int32, error) {
	return t.removeFunc(svc)
}

//nolint:gocognit // This returns a false positive due to the portmapper funcs defined in the test loop.
func TestShadowServiceManager_Create(t *testing.T) {
	tests := []struct {
		name        string
		provided    corev1.Service
		expected    corev1.Service
		findTCPPort int32
		addTCPPort  int32
		findUDPPort int32
		addUDPPort  int32
		expectedErr bool
	}{
		{
			name: "does not create when shadow service already exists",
			provided: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "alreadyexist",
					Namespace: "namespace",
				},
			},
			expected: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-alreadyexist-6d61657368-namespace",
					Namespace: "maesh",
					Labels: map[string]string{
						"app":               "maesh",
						"type":              "shadow",
						"test-alreadyexist": "true",
					},
				},
			},
		},
		{
			name: "create HTTP service by default",
			provided: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "http-default",
					Namespace: "namespace",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "portName",
							Protocol: corev1.ProtocolTCP,
							Port:     80,
						},
					},
				},
			},
			expected: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-http-default-6d61657368-namespace",
					Namespace: "maesh",
					Labels: map[string]string{
						"app":  "maesh",
						"type": "shadow",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "portName",
							Protocol:   corev1.ProtocolTCP,
							Port:       80,
							TargetPort: intstr.FromInt(5000),
						},
					},
					Selector: map[string]string{
						"component": "maesh-mesh",
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "create HTTP service",
			provided: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "http",
					Namespace: "namespace",
					Annotations: map[string]string{
						"maesh.containo.us/traffic-type": "http",
					}},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "portName",
							Protocol: corev1.ProtocolTCP,
							Port:     80,
						},
					},
				},
			},
			expected: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-http-6d61657368-namespace",
					Namespace: "maesh",
					Labels: map[string]string{
						"app":  "maesh",
						"type": "shadow",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "portName",
							Protocol:   corev1.ProtocolTCP,
							Port:       80,
							TargetPort: intstr.FromInt(5000),
						},
					},
					Selector: map[string]string{
						"component": "maesh-mesh",
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "create UDP service, reuse UDP port",
			provided: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "udp-reuse",
					Namespace: "namespace",
					Annotations: map[string]string{
						"maesh.containo.us/traffic-type": "udp",
					}},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "portName",
							Protocol: corev1.ProtocolUDP,
							Port:     80,
						},
					},
				},
			},
			expected: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-udp-reuse-6d61657368-namespace",
					Namespace: "maesh",
					Labels: map[string]string{
						"app":  "maesh",
						"type": "shadow",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "portName",
							Protocol:   corev1.ProtocolUDP,
							Port:       80,
							TargetPort: intstr.FromInt(10000),
						},
					},
					Selector: map[string]string{
						"component": "maesh-mesh",
					},
				},
			},
			findUDPPort: 10000,
			expectedErr: false,
		},
		{
			name: "create UDP service",
			provided: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "udp",
					Namespace: "namespace",
					Annotations: map[string]string{
						"maesh.containo.us/traffic-type": "udp",
					}},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "portName",
							Protocol: corev1.ProtocolUDP,
							Port:     80,
						},
					},
				},
			},
			expected: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-udp-6d61657368-namespace",
					Namespace: "maesh",
					Labels: map[string]string{
						"app":  "maesh",
						"type": "shadow",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "portName",
							Protocol:   corev1.ProtocolUDP,
							Port:       80,
							TargetPort: intstr.FromInt(10001),
						},
					},
					Selector: map[string]string{
						"component": "maesh-mesh",
					},
				},
			},
			addUDPPort:  10001,
			expectedErr: false,
		},
	}

	client, lister := makeClient(&corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "maesh-alreadyexist-6d61657368-namespace",
			Namespace: "maesh",
			Labels: map[string]string{
				"app":               "maesh",
				"type":              "shadow",
				"test-alreadyexist": "true",
			},
		},
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tcpPortMapper := portMapperMock{
				findFunc: func(svc k8s.ServiceWithPort) (i int32, b bool) {
					if test.findTCPPort != 0 {
						return test.findTCPPort, true
					}
					return 0, false
				},
				addFunc: func(svc *k8s.ServiceWithPort) (i int32, err error) {
					if test.addTCPPort != 0 {
						return test.addTCPPort, nil
					}
					return 0, errors.New("nope")
				},
			}
			udpPortMapper := portMapperMock{
				findFunc: func(svc k8s.ServiceWithPort) (i int32, b bool) {
					if test.findUDPPort != 0 {
						return test.findUDPPort, true
					}
					return 0, false
				},
				addFunc: func(svc *k8s.ServiceWithPort) (i int32, err error) {
					if test.addUDPPort != 0 {
						return test.addUDPPort, nil
					}
					return 0, errors.New("nope")
				},
			}

			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			service := controller.NewShadowServiceManager(log, lister, "maesh", tcpPortMapper, udpPortMapper, "http", 5000, 5002, client)
			err := service.Create(&test.provided)
			if test.expectedErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			svcGot, err := client.CoreV1().Services("maesh").Get(test.expected.Name, v1.GetOptions{})
			assert.NoError(t, err)

			assert.Equal(t, &test.expected, svcGot)
		})
	}
}

func TestShadowServiceManager_Update(t *testing.T) {
	tests := []struct {
		name              string
		defaultMode       string
		portProtocol      corev1.Protocol
		tcpPortMapperMock func(*k8s.ServiceWithPort, *k8s.ServiceWithPort) portMapperMock
		udpPortMapperMock func(*k8s.ServiceWithPort, *k8s.ServiceWithPort) portMapperMock
	}{
		{
			name:         "update TCP service",
			defaultMode:  "tcp",
			portProtocol: corev1.ProtocolTCP,
			udpPortMapperMock: func(addedPortMapping, removedPortMapping *k8s.ServiceWithPort) portMapperMock {
				return portMapperMock{}
			},
			tcpPortMapperMock: func(addedPortMapping, removedPortMapping *k8s.ServiceWithPort) portMapperMock {
				return portMapperMock{
					findFunc: func(svc k8s.ServiceWithPort) (int32, bool) {
						return 0, false
					},
					addFunc: func(svc *k8s.ServiceWithPort) (int32, error) {
						*addedPortMapping = *svc
						return 10001, nil
					},
					removeFunc: func(svc k8s.ServiceWithPort) (int32, error) {
						*removedPortMapping = svc
						return 10001, nil
					},
				}
			},
		},
		{
			name:         "update UDP service",
			defaultMode:  "udp",
			portProtocol: corev1.ProtocolUDP,
			tcpPortMapperMock: func(addedPortMapping, removedPortMapping *k8s.ServiceWithPort) portMapperMock {
				return portMapperMock{}
			},
			udpPortMapperMock: func(addedPortMapping, removedPortMapping *k8s.ServiceWithPort) portMapperMock {
				return portMapperMock{
					findFunc: func(svc k8s.ServiceWithPort) (int32, bool) {
						return 0, false
					},
					addFunc: func(svc *k8s.ServiceWithPort) (int32, error) {
						*addedPortMapping = *svc
						return 10001, nil
					},
					removeFunc: func(svc k8s.ServiceWithPort) (int32, error) {
						*removedPortMapping = svc
						return 10001, nil
					},
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			shadowSvc := &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-my-svc-6d61657368-my-ns",
					Namespace: "maesh",
					Labels: map[string]string{
						"app":  "maesh",
						"type": "shadow",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "portName",
							Protocol:   test.portProtocol,
							Port:       8080,
							TargetPort: intstr.FromInt(10001),
						},
					},
					Selector: map[string]string{
						"component": "maesh-mesh",
					},
				},
			}
			client, lister := makeClient(shadowSvc)

			var addedPortMapping, removedPortMapping k8s.ServiceWithPort

			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			service := controller.NewShadowServiceManager(
				log, lister,
				"maesh",
				test.tcpPortMapperMock(&addedPortMapping, &removedPortMapping),
				test.udpPortMapperMock(&addedPortMapping, &removedPortMapping),
				test.defaultMode,
				5000,
				5002,
				client,
			)

			oldUserSvc := corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "my-svc",
					Namespace: "my-ns",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "portName",
							Protocol: test.portProtocol,
							Port:     8090,
						},
					},
				},
			}
			newUserSvc := corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "my-svc",
					Namespace: "my-ns",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "portName",
							Protocol: test.portProtocol,
							Port:     80,
						},
					},
				},
			}
			expected := corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-my-svc-6d61657368-my-ns",
					Namespace: "maesh",
					Labels: map[string]string{
						"app":  "maesh",
						"type": "shadow",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "portName",
							Protocol:   test.portProtocol,
							Port:       80,
							TargetPort: intstr.FromInt(10001),
						},
					},
					Selector: map[string]string{
						"component": "maesh-mesh",
					},
				},
			}
			svcGot, err := service.Update(&oldUserSvc, &newUserSvc)
			require.NoError(t, err)
			assert.Equal(t, &expected, svcGot)

			assert.Equal(t, k8s.ServiceWithPort{
				Namespace: oldUserSvc.Namespace,
				Name:      oldUserSvc.Name,
				Port:      oldUserSvc.Spec.Ports[0].Port,
			}, removedPortMapping)
			assert.Equal(t, k8s.ServiceWithPort{
				Namespace: newUserSvc.Namespace,
				Name:      newUserSvc.Name,
				Port:      newUserSvc.Spec.Ports[0].Port,
			}, addedPortMapping)

			svcGot, err = client.CoreV1().Services("maesh").Get(expected.Name, v1.GetOptions{})
			assert.NoError(t, err)

			assert.Equal(t, &expected, svcGot)
		})
	}
}

func TestShadowServiceManager_Delete(t *testing.T) {
	tests := []struct {
		name              string
		defaultMode       string
		portProtocol      corev1.Protocol
		tcpPortMapperMock func(*k8s.ServiceWithPort) portMapperMock
		udpPortMapperMock func(*k8s.ServiceWithPort) portMapperMock
	}{
		{
			name:         "delete TCP service",
			defaultMode:  "tcp",
			portProtocol: corev1.ProtocolTCP,
			udpPortMapperMock: func(removedPortMapping *k8s.ServiceWithPort) portMapperMock {
				return portMapperMock{}
			},
			tcpPortMapperMock: func(removedPortMapping *k8s.ServiceWithPort) portMapperMock {
				return portMapperMock{
					removeFunc: func(svc k8s.ServiceWithPort) (int32, error) {
						*removedPortMapping = svc
						return 10001, nil
					},
				}
			},
		},
		{
			name:         "delete UDP service",
			defaultMode:  "udp",
			portProtocol: corev1.ProtocolUDP,
			tcpPortMapperMock: func(removedPortMapping *k8s.ServiceWithPort) portMapperMock {
				return portMapperMock{}
			},
			udpPortMapperMock: func(removedPortMapping *k8s.ServiceWithPort) portMapperMock {
				return portMapperMock{
					removeFunc: func(svc k8s.ServiceWithPort) (int32, error) {
						*removedPortMapping = svc
						return 10001, nil
					},
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var removedPortMapping k8s.ServiceWithPort

			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			shadowSvc := &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-my-svc-6d61657368-my-ns",
					Namespace: "maesh",
					Labels: map[string]string{
						"app":  "maesh",
						"type": "shadow",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "portName",
							Protocol:   test.portProtocol,
							Port:       8088,
							TargetPort: intstr.FromInt(10001),
						},
					},
					Selector: map[string]string{
						"component": "maesh-mesh",
					},
				},
			}
			client, lister := makeClient(shadowSvc)

			service := controller.NewShadowServiceManager(
				log,
				lister,
				"maesh",
				test.tcpPortMapperMock(&removedPortMapping),
				test.udpPortMapperMock(&removedPortMapping),
				test.defaultMode,
				5000,
				5002,
				client,
			)

			userSvc := &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "my-svc",
					Namespace: "my-ns",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "portName",
							Protocol: test.portProtocol,
							Port:     8088,
						},
					},
				},
			}
			err := service.Delete(userSvc)
			require.NoError(t, err)

			assert.Equal(t, k8s.ServiceWithPort{
				Namespace: userSvc.Namespace,
				Name:      userSvc.Name,
				Port:      userSvc.Spec.Ports[0].Port,
			}, removedPortMapping)

			_, err = client.CoreV1().Services("maesh").Get(shadowSvc.Name, v1.GetOptions{})
			assert.Error(t, err)
		})
	}
}

func makeClient(args ...runtime.Object) (*fake.Clientset, listers.ServiceLister) {
	client := fake.NewSimpleClientset(args...)

	informerFactory := informers.NewSharedInformerFactory(client, 5*time.Minute)
	lister := informerFactory.Core().V1().Services().Lister()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stop := make(<-chan struct{})
	informerFactory.Start(stop)
	informerFactory.WaitForCacheSync(ctx.Done())

	return client, lister
}
