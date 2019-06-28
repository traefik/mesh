package kubernetes

import (
	"testing"

	"github.com/containous/traefik/pkg/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildRouterFromService(t *testing.T) {
	testService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "foo",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.0.0.1",
		},
	}

	expected := &config.Router{
		Rule: "Host(`test.foo.traefik.mesh`) || Host(`10.0.0.1`)",
	}

	provider := New(nil)

	actual := provider.buildRouterFromService(testService)

	assert.Equal(t, expected, actual)
}
