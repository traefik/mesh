package controller

import (
	"context"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	listers "k8s.io/client-go/listers/core/v1"
)

func TestPortMapping_AddEmptyState(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	serviceLister, err := newFakeServiceLister()
	require.NoError(t, err)

	p := NewPortMapping("maesh", serviceLister, logger, 10000, 10200)

	wantSp := &k8s.ServicePort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}
	port, err := p.Add(wantSp)
	require.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	gotSp := p.table[10000]
	require.NotNil(t, gotSp)
	assert.Equal(t, wantSp, gotSp)
}

func TestPortMapping_AddOverflow(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	serviceLister, err := newFakeServiceLister()
	require.NoError(t, err)

	p := NewPortMapping("maesh", serviceLister, logger, 10000, 10001)

	wantSp := &k8s.ServicePort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}

	port, err := p.Add(wantSp)
	require.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	port, err = p.Add(wantSp)
	require.NoError(t, err)
	assert.Equal(t, int32(10001), port)

	_, err = p.Add(wantSp)
	assert.Error(t, err)

	gotSp := p.table[10000]
	require.NotNil(t, gotSp)
	assert.Equal(t, wantSp, gotSp)

	gotSp = p.table[10001]
	require.NotNil(t, gotSp)
	assert.Equal(t, wantSp, gotSp)

	gotSp = p.table[10002]
	assert.Nil(t, gotSp)
}

func TestPortMapping_FindWithState(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	serviceLister, err := newFakeServiceLister()
	require.NoError(t, err)

	p := NewPortMapping("maesh", serviceLister, logger, 10000, 10200)

	p.table[10000] = &k8s.ServicePort{Namespace: "my-ns", Name: "my-app", Port: 9090}
	p.table[10002] = &k8s.ServicePort{Namespace: "my-ns", Name: "my-app2", Port: 9092}

	sp := k8s.ServicePort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}
	port, ok := p.Find(sp)
	require.True(t, ok)
	assert.Equal(t, int32(10000), port)

	sp = k8s.ServicePort{
		Namespace: "my-ns2",
		Name:      "my-app",
		Port:      9090,
	}
	_, ok = p.Find(sp)
	assert.False(t, ok)
}

func TestPortMapping_Remove(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	serviceLister, err := newFakeServiceLister()
	require.NoError(t, err)

	p := NewPortMapping("maesh", serviceLister, logger, 10000, 10200)

	p.table[10000] = &k8s.ServicePort{Namespace: "my-ns", Name: "my-app", Port: 9090}

	sp := k8s.ServicePort{
		Namespace: "my-ns",
		Name:      "my-app",
		Port:      9090,
	}
	port, err := p.Remove(sp)
	require.NoError(t, err)
	assert.Equal(t, int32(10000), port)

	_, err = p.Remove(sp)
	assert.Error(t, err)

	unknownSp := k8s.ServicePort{
		Namespace: "my-unknown-ns",
		Name:      "my-unknown-app",
		Port:      8088,
	}
	_, err = p.Remove(unknownSp)
	assert.Error(t, err)
}

func TestPortMapping_LoadState(t *testing.T) {
	tests := []struct {
		desc     string
		services []runtime.Object
		expPorts []int32
	}{
		{
			desc: "should be empty if there is no shadow services",
		},
		{
			desc:     "should ignore shadow services with malformed shadowSvcName",
			expPorts: []int32{10001},
			services: []runtime.Object{
				newShadowService("foo", corev1.ServicePort{
					Port:       80,
					TargetPort: intstr.FromInt(10000),
				}),
				newShadowService("maesh-foo-6d61657368-maesh", corev1.ServicePort{
					Port:       80,
					TargetPort: intstr.FromInt(10001),
				}),
			},
		},
		{
			desc:     "should ignore the shadow service ports with an out of range target port",
			expPorts: []int32{10001},
			services: []runtime.Object{
				newShadowService("maesh-foo-6d61657368-maesh",
					corev1.ServicePort{
						Port:       80,
						TargetPort: intstr.FromInt(5000),
					}, corev1.ServicePort{
						Port:       8080,
						TargetPort: intstr.FromInt(10001),
					}),
			},
		},
		{
			desc:     "should initialize the state with all the shadow service target ports",
			expPorts: []int32{10000, 10001, 10002, 10003},
			services: []runtime.Object{
				newShadowService("maesh-foo-6d61657368-maesh",
					corev1.ServicePort{
						Port:       80,
						TargetPort: intstr.FromInt(10002),
					}, corev1.ServicePort{
						Port:       8080,
						TargetPort: intstr.FromInt(10003),
					}),
				newShadowService("maesh-bar-6d61657368-maesh",
					corev1.ServicePort{
						Port:       80,
						TargetPort: intstr.FromInt(10000),
					}, corev1.ServicePort{
						Port:       8080,
						TargetPort: intstr.FromInt(10001),
					}),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			logger := logrus.New()
			logger.SetOutput(ioutil.Discard)

			serviceLister, err := newFakeServiceLister(test.services...)
			require.NoError(t, err)

			portMapping := NewPortMapping("maesh", serviceLister, logger, 10000, 10005)

			err = portMapping.LoadState()

			require.NoError(t, err)
			assert.Equal(t, len(test.expPorts), len(portMapping.table))

			for _, port := range test.expPorts {
				_, exists := portMapping.table[port]
				require.True(t, exists)
			}
		})
	}
}

func TestPortMapping_parseServiceNamespaceAndName(t *testing.T) {
	tests := []struct {
		desc          string
		shadowSvcName string
		expErr        bool
		expNamespace  string
		expName       string
	}{
		{
			desc:          "should return an error if the shadow service name is malformed",
			shadowSvcName: "foo",
			expErr:        true,
		},
		{
			desc:          "should return the parsed service namespace and name from the shadow service name",
			shadowSvcName: "maesh-foo-6d61657368-default",
			expNamespace:  "default",
			expName:       "foo",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			logger := logrus.New()
			logger.SetOutput(ioutil.Discard)

			serviceLister, err := newFakeServiceLister()
			require.NoError(t, err)

			portMapping := NewPortMapping("maesh", serviceLister, logger, 10000, 10005)

			namespace, name, err := portMapping.parseServiceNamespaceAndName(test.shadowSvcName)
			if test.expErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expNamespace, namespace)
			assert.Equal(t, test.expName, name)
		})
	}
}

func newFakeServiceLister(services ...runtime.Object) (listers.ServiceLister, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := fake.NewSimpleClientset(services...)
	factory := informers.NewSharedInformerFactory(client, k8s.ResyncPeriod)
	serviceLister := factory.Core().V1().Services().Lister()

	factory.Start(ctx.Done())

	for t, ok := range factory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return nil, fmt.Errorf("timed out while waiting for cache sync: %s", t.String())
		}
	}

	return serviceLister, nil
}

func newShadowService(name string, ports ...corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "maesh",
			Name:      name,
			Labels: map[string]string{
				"app":  "maesh",
				"type": "shadow",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: ports,
		},
	}
}
