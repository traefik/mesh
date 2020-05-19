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

func TestCleanup_New(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientMock := k8s.NewClientMock(t, ctx.Done(), "mock.yaml", false)
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	cln := NewCleanup(log, clientMock, metav1.NamespaceDefault)
	assert.NotNil(t, cln)
}

func TestCleanup_CleanShadowServices(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientMock := k8s.NewClientMock(t, ctx.Done(), "mock.yaml", false)
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	cln := NewCleanup(log, clientMock, "maesh")
	assert.NotNil(t, cln)

	err := cln.CleanShadowServices()
	assert.NoError(t, err)

	sl, err := clientMock.KubernetesClient().CoreV1().Services(metav1.NamespaceAll).List(metav1.ListOptions{
		LabelSelector: "app=maesh,type=shadow",
	})
	assert.NoError(t, err)
	assert.Len(t, sl.Items, 0)

	srv, err := clientMock.KubernetesClient().CoreV1().Services(metav1.NamespaceAll).List(metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Len(t, srv.Items, 2)
}

func TestCleanup_RestoreDNSConfig(t *testing.T) {
	tests := []struct {
		desc          string
		mockFile      string
		configmapname string
	}{
		{
			desc:          "Restore CoreDNS",
			mockFile:      "coredns_patched.yaml",
			configmapname: "coredns-cfgmap",
		},
		{
			desc:          "Restore KubeDNS",
			mockFile:      "kubedns_patched.yaml",
			configmapname: "kubedns-cfgmap",
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clientMock := k8s.NewClientMock(t, ctx.Done(), test.mockFile, false)
			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			cln := NewCleanup(log, clientMock, "maesh")
			assert.NotNil(t, cln)

			err := cln.RestoreDNSConfig()
			assert.NoError(t, err)

			cl, err := clientMock.KubernetesClient().CoreV1().ConfigMaps("maesh").List(metav1.ListOptions{})
			assert.NoError(t, err)
			assert.Len(t, cl.Items, 0)

			cfg, err := clientMock.KubernetesClient().CoreV1().ConfigMaps(metav1.NamespaceSystem).Get(test.configmapname, metav1.GetOptions{})
			assert.NoError(t, err)
			assert.Len(t, cfg.ObjectMeta.Labels, 0)
		})
	}
}
