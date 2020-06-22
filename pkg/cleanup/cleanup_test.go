package cleanup

import (
	"context"
	"os"
	"testing"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCleanup_New(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientMock := k8s.NewClientMock(t, ctx.Done(), "mock.yaml", false)
	logger := logrus.New()

	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.DebugLevel)

	cleanup := NewCleanup(logger, clientMock.KubernetesClient(), metav1.NamespaceDefault)
	require.NotNil(t, cleanup)
}

func TestCleanup_CleanShadowServices(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientMock := k8s.NewClientMock(t, ctx.Done(), "mock.yaml", false)
	logger := logrus.New()

	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.DebugLevel)

	cleanup := NewCleanup(logger, clientMock.KubernetesClient(), "maesh")
	require.NotNil(t, cleanup)

	err := cleanup.CleanShadowServices()
	require.NoError(t, err)

	serviceList, err := clientMock.KubernetesClient().CoreV1().Services(metav1.NamespaceAll).List(metav1.ListOptions{
		LabelSelector: "app=maesh,type=shadow",
	})
	require.NoError(t, err)
	assert.Len(t, serviceList.Items, 0)

	serviceList, err = clientMock.KubernetesClient().CoreV1().Services(metav1.NamespaceAll).List(metav1.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, serviceList.Items, 2)
}
