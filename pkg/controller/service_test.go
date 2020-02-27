package controller_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/containous/maesh/pkg/controller"
	"github.com/containous/maesh/pkg/k8s"
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

type tcpPortMapperMock struct {
	findFunc func(svc k8s.ServiceWithPort) (int32, bool)
	getFunc  func(srcPort int32) *k8s.ServiceWithPort
	addFunc  func(svc *k8s.ServiceWithPort) (int32, error)
}

func (t tcpPortMapperMock) Find(svc k8s.ServiceWithPort) (int32, bool) {
	return t.findFunc(svc)
}

func (t tcpPortMapperMock) Get(srcPort int32) *k8s.ServiceWithPort {
	return t.getFunc(srcPort)
}

func (t tcpPortMapperMock) Add(svc *k8s.ServiceWithPort) (int32, error) {
	return t.addFunc(svc)
}

func Test_ServiceCreate(t *testing.T) {
	tests := []struct {
		name     string
		input    corev1.Service
		want     corev1.Service
		findPort int32
		addPort  int32
		wantErr  bool
	}{
		{
			name: "does not create when shadow service already exists",
			input: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "alreadyexist",
					Namespace: "namespace",
				},
			},
			want: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-alreadyexist-6d61657368-namespace",
					Namespace: "maesh",
					Labels: map[string]string{
						"app":               "maesh",
						"test-alreadyexist": "true",
					},
				},
			},
		},
		{
			name: "create HTTP service by default",
			input: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "http-default",
					Namespace: "namespace",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "portName", Protocol: corev1.ProtocolTCP, Port: 80},
					},
				},
			},
			want: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-http-default-6d61657368-namespace",
					Namespace: "maesh",
					Labels: map[string]string{
						"app": "maesh",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "portName", Protocol: corev1.ProtocolTCP, Port: 80, TargetPort: intstr.FromInt(5000)},
					},
					Selector: map[string]string{
						"component": "maesh-mesh",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "create HTTP service",
			input: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "http",
					Namespace: "namespace",
					Annotations: map[string]string{
						"maesh.containo.us/traffic-type": "http",
					}},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "portName", Protocol: corev1.ProtocolTCP, Port: 80},
					},
				},
			},
			want: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-http-6d61657368-namespace",
					Namespace: "maesh",
					Labels: map[string]string{
						"app": "maesh",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "portName", Protocol: corev1.ProtocolTCP, Port: 80, TargetPort: intstr.FromInt(5000)},
					},
					Selector: map[string]string{
						"component": "maesh-mesh",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "create TCP service, reuse TCP port",
			input: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "tcp-reuse",
					Namespace: "namespace",
					Annotations: map[string]string{
						"maesh.containo.us/traffic-type": "tcp",
					}},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "portName", Protocol: corev1.ProtocolTCP, Port: 80},
					},
				},
			},
			want: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-tcp-reuse-6d61657368-namespace",
					Namespace: "maesh",
					Labels: map[string]string{
						"app": "maesh",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "portName", Protocol: corev1.ProtocolTCP, Port: 80, TargetPort: intstr.FromInt(10000)},
					},
					Selector: map[string]string{
						"component": "maesh-mesh",
					},
				},
			},
			findPort: 10000,
			wantErr:  false,
		},
		{
			name: "create TCP service",
			input: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "tcp",
					Namespace: "namespace",
					Annotations: map[string]string{
						"maesh.containo.us/traffic-type": "tcp",
					}},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "portName", Protocol: corev1.ProtocolTCP, Port: 80},
					},
				},
			},
			want: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-tcp-6d61657368-namespace",
					Namespace: "maesh",
					Labels: map[string]string{
						"app": "maesh",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "portName", Protocol: corev1.ProtocolTCP, Port: 80, TargetPort: intstr.FromInt(10001)},
					},
					Selector: map[string]string{
						"component": "maesh-mesh",
					},
				},
			},
			addPort: 10001,
			wantErr: false,
		},
	}

	client, lister := makeClient(&corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "maesh-alreadyexist-6d61657368-namespace",
			Namespace: "maesh",
			Labels: map[string]string{
				"app":               "maesh",
				"test-alreadyexist": "true",
			},
		},
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tcpPortMapper := tcpPortMapperMock{
				findFunc: func(svc k8s.ServiceWithPort) (i int32, b bool) {
					if test.findPort != 0 {
						return test.findPort, true
					}
					return 0, false
				},
				addFunc: func(svc *k8s.ServiceWithPort) (i int32, err error) {
					if test.addPort != 0 {
						return test.addPort, nil
					}
					return 0, errors.New("nope")
				},
			}

			service := controller.NewShadowServiceManager(lister, "maesh", tcpPortMapper, "http", 5000, 5002, client)
			err := service.Create(&test.input)
			if test.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			svcGot, err := client.CoreV1().Services("maesh").Get(test.want.Name, v1.GetOptions{})
			assert.NoError(t, err)

			assert.Equal(t, &test.want, svcGot)
		})
	}
}

func Test_ServiceUpdate(t *testing.T) {
	tests := []struct {
		name     string
		input    corev1.Service
		want     corev1.Service
		findPort int32
		addPort  int32
		wantErr  bool
	}{
		{
			name: "create HTTP service by default",
			input: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "alreadyexist",
					Namespace: "namespace",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "portName", Protocol: corev1.ProtocolTCP, Port: 80},
					},
				},
			},
			want: corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "maesh-alreadyexist-6d61657368-namespace",
					Namespace: "maesh",
					Labels: map[string]string{
						"app": "maesh",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "portName", Protocol: corev1.ProtocolTCP, Port: 80, TargetPort: intstr.FromInt(5000)},
					},
					Selector: map[string]string{
						"component": "maesh-mesh",
					},
				},
			},
			wantErr: false,
		},
	}

	client, lister := makeClient(&corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "maesh-alreadyexist-6d61657368-namespace",
			Namespace: "maesh",
			Labels: map[string]string{
				"app": "maesh",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "portName", Protocol: corev1.ProtocolTCP, Port: 8080, TargetPort: intstr.FromInt(5000)},
			},
			Selector: map[string]string{
				"component": "maesh-mesh",
			},
		},
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tcpPortMapper := tcpPortMapperMock{
				findFunc: func(svc k8s.ServiceWithPort) (i int32, b bool) {
					if test.findPort != 0 {
						return test.findPort, true
					}
					return 0, false
				},
				addFunc: func(svc *k8s.ServiceWithPort) (i int32, err error) {
					if test.addPort != 0 {
						return test.addPort, nil
					}
					return 0, errors.New("nope")
				},
			}

			service := controller.NewShadowServiceManager(lister, "maesh", tcpPortMapper, "http", 5000, 5002, client)

			svcGot, err := service.Update(&test.input)
			if test.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, &test.want, svcGot)
		})
	}
}

func Test_ServiceDelete(t *testing.T) {
	client, lister := makeClient(&corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "maesh-alreadyexist-6d61657368-namespace",
			Namespace: "maesh",
			Labels: map[string]string{
				"app": "maesh",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "portName", Protocol: corev1.ProtocolTCP, Port: 8080, TargetPort: intstr.FromInt(5000)},
			},
			Selector: map[string]string{
				"component": "maesh-mesh",
			},
		},
	})

	service := controller.NewShadowServiceManager(lister, "maesh", nil, "http", 5000, 5002, client)
	err := service.Delete("alreadyexist", "namespace")
	require.NoError(t, err)

	_, err = client.CoreV1().Services("maesh").Get("maesh-alreadyexist-6d61657368-namespace", v1.GetOptions{})
	assert.Error(t, err)
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
