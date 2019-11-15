package mesher_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/containous/maesh/internal/mesher"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

const meshNamespace = "maesh"

func TestController(t *testing.T) {
	tests := []struct {
		name            string
		createResources func(*testing.T, *fake.Clientset)
		updateResources func(*testing.T, *fake.Clientset)

		wantService             *corev1.Service
		wantMeshServiceDeletion bool
	}{
		{
			name: "creates mesh service on add",
			createResources: func(t *testing.T, c *fake.Clientset) {
				t.Helper()
				_, err := c.CoreV1().Services("app").Create(generateAppService(8080))
				require.NoError(t, err)
			},
			wantService: generateMeshService(8080),
		},
		{
			name: "update mesh service on add if it already exists",
			createResources: func(t *testing.T, c *fake.Clientset) {
				t.Helper()
				_, err := c.CoreV1().Services("app").Create(generateAppService(8080))
				require.NoError(t, err)

				_, err = c.CoreV1().Services(meshNamespace).Create(generateMeshService(9090))
				require.NoError(t, err)
			},
			wantService: generateMeshService(8080),
		},
		{
			name: "updates mesh service",
			createResources: func(t *testing.T, c *fake.Clientset) {
				t.Helper()
				_, err := c.CoreV1().Services("app").Create(generateAppService(9999))
				require.NoError(t, err)
			},
			updateResources: func(t *testing.T, c *fake.Clientset) {
				_, err := c.CoreV1().Services("app").Update(generateAppService(8080))
				require.NoError(t, err)
			},
			wantService: generateMeshService(8080),
		},
		{
			name: "creates mesh service on update if deleted",
			createResources: func(t *testing.T, c *fake.Clientset) {
				t.Helper()
				_, err := c.CoreV1().Services("app").Create(generateAppService(9999))
				require.NoError(t, err)
			},
			updateResources: func(t *testing.T, c *fake.Clientset) {
				t.Helper()
				err := c.CoreV1().Services(meshNamespace).Delete("maesh-foo-app", nil)
				require.NoError(t, err)

				_, err = c.CoreV1().Services("app").Update(generateAppService(8080))
				require.NoError(t, err)
			},
			wantService: generateMeshService(8080),
		},
		{
			name: "deletes mesh service",
			createResources: func(t *testing.T, c *fake.Clientset) {
				t.Helper()
				_, err := c.CoreV1().Services("app").Create(generateAppService(8080))
				require.NoError(t, err)
			},
			updateResources: func(t *testing.T, c *fake.Clientset) {
				t.Helper()
				err := c.CoreV1().Services("app").Delete("foo", nil)
				require.NoError(t, err)
			},
			wantMeshServiceDeletion: true,
			wantService:             generateMeshService(8080),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()

			clientSet := fake.NewSimpleClientset()
			factory := informers.NewSharedInformerFactory(clientSet, 0)

			controller := mesher.NewController(clientSet, factory, meshNamespace, logger())

			factory.Start(ctx.Done())

			test.createResources(t, clientSet)
			factory.WaitForCacheSync(ctx.Done())

			go controller.Run()

			// Forces the processing of the createResources events by the controller.
			time.Sleep(time.Millisecond)

			if test.updateResources != nil {
				test.updateResources(t, clientSet)
				factory.WaitForCacheSync(ctx.Done())
			}

			controller.ShutDown()
			_ = controller.Wait(ctx)

			if test.wantMeshServiceDeletion {
				assertMeshServiceDeletion(t, clientSet, test.wantService)
				return
			}

			assertHasService(t, clientSet, test.wantService)
		})
	}
}

func generateAppService(port int32) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "app",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       port,
					TargetPort: intstr.FromInt(8080),
				},
			},

			Selector: map[string]string{
				"app": "foo",
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

func generateMeshService(port int32) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "maesh-foo-app",
			Namespace: meshNamespace,
			Labels: map[string]string{
				"app":       "maesh",
				"component": "mesh-svc",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       port,
					TargetPort: intstr.FromInt(5000),
				},
			},
			Selector: map[string]string{
				"app":       "maesh",
				"component": "mesh-node",
			},
		},
	}
}

func updateRessourceDeleteService(t *testing.T, c *fake.Clientset) {
	t.Helper()
	err := c.CoreV1().Services("app").Delete("foo", &metav1.DeleteOptions{})
	require.NoError(t, err)
}

func assertHasService(t *testing.T, c *fake.Clientset, want *corev1.Service) {
	t.Helper()

	got, err := c.CoreV1().Services(want.GetNamespace()).Get(want.GetName(), metav1.GetOptions{})
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, want, got)
}

func assertMeshServiceDeletion(t *testing.T, c *fake.Clientset, svc *corev1.Service) {
	t.Helper()

	_, err := c.CoreV1().Services(meshNamespace).Get(svc.GetName(), metav1.GetOptions{})
	assert.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func logger() logrus.FieldLogger {
	l := logrus.New()
	l.SetOutput(os.Stdout)
	l.SetLevel(logrus.InfoLevel)
	return l
}
