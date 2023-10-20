package cleanup

import (
	"context"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/mesh/pkg/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCleanup_New(t *testing.T) {
	clientMock := k8s.NewClientMock("mock.yaml")
	logger := logrus.New()

	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.DebugLevel)

	cleanup := NewCleanup(logger, clientMock.KubernetesClient(), metav1.NamespaceDefault)
	require.NotNil(t, cleanup)
}

func TestCleanup_CleanShadowServices(t *testing.T) {
	clientMock := k8s.NewClientMock("mock.yaml")
	logger := logrus.New()

	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.DebugLevel)

	cleanup := NewCleanup(logger, clientMock.KubernetesClient(), "traefik-mesh")
	require.NotNil(t, cleanup)

	err := cleanup.CleanShadowServices(context.Background())
	require.NoError(t, err)

	serviceList, err := clientMock.KubernetesClient().CoreV1().Services(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{
		LabelSelector: "app=maesh,type=shadow",
	})
	require.NoError(t, err)
	assert.Empty(t, serviceList.Items)

	serviceList, err = clientMock.KubernetesClient().CoreV1().Services(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, serviceList.Items, 2)
}
