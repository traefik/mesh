package cleanup

import (
	"context"
	"os"
	"testing"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientMock := k8s.NewClientMock(ctx.Done(), "mock.yaml", false)
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	cln := NewCleanup(log, clientMock)
	assert.NotNil(t, cln)
}

func TestCleanShadowServices(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientMock := k8s.NewClientMock(ctx.Done(), "mock.yaml", false)
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	cln := NewCleanup(log, clientMock)
	assert.NotNil(t, cln)

	err := cln.CleanShadowServices()
	assert.NoError(t, err)

	sl, err := clientMock.GetKubernetesClient().CoreV1().Services(metav1.NamespaceAll).List(metav1.ListOptions{
		LabelSelector: "app=maesh",
	})
	assert.NoError(t, err)
	assert.Len(t, sl.Items, 0)

	srv, err := clientMock.GetKubernetesClient().CoreV1().Services(metav1.NamespaceAll).List(metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Len(t, srv.Items, 1)

}
