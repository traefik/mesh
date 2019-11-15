package mesher_test

import (
	"context"
	"testing"

	"github.com/containous/maesh/internal/mesher"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func assertHasService(t *testing.T, c *fake.Clientset, want *corev1.Service) {
	t.Helper()

	got, err := c.CoreV1().Services(want.GetNamespace()).Get(want.GetName(), metav1.GetOptions{})
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, want, got)
}

func TestController(t *testing.T) {
	tests := []struct {
		name            string
		createResources func(*testing.T, *fake.Clientset)

		wantService *corev1.Service
	}{
		{
			name: "creates mesh service",
			createResources: func(t *testing.T, c *fake.Clientset) {
				t.Helper()
				c.CoreV1().Services("test").Create(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "svc",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name:       "http",
								Protocol:   corev1.ProtocolTCP,
								Port:       8080,
								TargetPort: intstr.FromInt(8080),
							},
						},
						Selector: map[string]string{
							"app": "foo",
						},
						Type: corev1.ServiceTypeClusterIP,
					},
				})
			},
			wantService: &corev1.Service{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()

			clientSet := fake.NewSimpleClientset()
			factory := informers.NewSharedInformerFactory(clientSet, 0)

			factory.Start(ctx.Done())
			factory.WaitForCacheSync(ctx.Done())

			controller := mesher.NewController(
				clientSet.CoreV1().Services(metav1.NamespaceAll),
				factory.Core().V1().Services(),
				nil, // TODO ?
			)

			go controller.Run()

			test.createResources(t, clientSet)
			controller.ShutDown()
			controller.Wait(ctx)

			assertHasService(t, clientSet, test.wantService)
		})
	}
}
