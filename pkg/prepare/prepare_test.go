package prepare_test

import (
	"context"
	"testing"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/prepare"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckDNSProvider(t *testing.T) {
	tests := []struct {
		desc string

		mockFile string

		expectedProvider prepare.DNSProvider
		expectedErr      bool
	}{
		{
			desc: "CoreDNS supported version",

			mockFile: "checkdnsprovider_supported_version.yaml",

			expectedProvider: prepare.CoreDNS,
			expectedErr:      false,
		},
		{
			desc: "KubeDNS",

			mockFile: "checkdnsprovider_kubedns.yaml",

			expectedProvider: prepare.KubeDNS,
			expectedErr:      false,
		},
		{
			desc: "CoreDNS unsupported version",

			mockFile: "checkdnsprovider_unsupported_version.yaml",

			expectedProvider: prepare.UnknownDNS,
			expectedErr:      true,
		},
		{
			desc: "No known DNS provider",

			mockFile: "checkdnsprovider_no_provider.yaml",

			expectedProvider: prepare.UnknownDNS,
			expectedErr:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clt := k8s.NewClientMock(t, ctx.Done(), test.mockFile, false)

			prep := prepare.NewPrepare(logrus.New(), clt)
			provider, err := prep.CheckDNSProvider()

			if test.expectedErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, test.expectedProvider, provider)
		})
	}
}

func TestConfigureCoreDNS(t *testing.T) {
	tests := []struct {
		desc string

		mockFile string

		expectedCorefile string
		expectedErr      bool
	}{
		{
			desc: "First time config of CoreDNS",

			mockFile: "configurecoredns_not_patched.yaml",

			expectedErr:      false,
			expectedCorefile: ".:53 {\n    errors\n    health {\n        lameduck 5s\n    }\n    ready\n    kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n        pods insecure\n        fallthrough in-addr.arpa ip6.arpa\n        ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n    \tfallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n",
		},
		{
			desc: "Missing Corefile configmap",

			mockFile: "configurecoredns_missing_configmap.yaml",

			expectedErr: true,
		},
		{
			desc: "Missing CoreDNS deployment",

			mockFile: "configurecoredns_missing_deployment.yaml",

			expectedErr: true,
		},
		{
			desc: "Already patched",

			mockFile: "configurecoredns_already_patched.yaml",

			expectedErr:      false,
			expectedCorefile: ".:53 {\n        errors\n        health {\n            lameduck 5s\n        }\n        ready\n        kubernetes {{ pillar['dns_domain'] }} in-addr.arpa ip6.arpa {\n            pods insecure\n            fallthrough in-addr.arpa ip6.arpa\n            ttl 30\n        }\n        prometheus :9153\n        forward . /etc/resolv.conf\n        cache 30\n        loop\n        reload\n        loadbalance\n    }\nmaesh:53 {\n    errors\n    rewrite continue {\n        name regex ([a-zA-Z0-9-_]*)\\.([a-zv0-9-_]*)\\.maesh toto-{1}-6d61657368-{2}.toto.svc.titi\n        answer name toto-([a-zA-Z0-9-_]*)-6d61657368-([a-zA-Z0-9-_]*)\\.toto\\.svc\\.titi {1}.{2}.maesh\n    }\n    kubernetes titi in-addr.arpa ip6.arpa {\n        pods insecure\n        upstream\n    \tfallthrough in-addr.arpa ip6.arpa\n    }\n    forward . /etc/resolv.conf\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clt := k8s.NewClientMock(t, ctx.Done(), test.mockFile, false)

			prep := prepare.NewPrepare(logrus.New(), clt)
			err := prep.ConfigureCoreDNS("titi", "toto")
			if test.expectedErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			cfgMap, err := clt.KubernetesClient().CoreV1().ConfigMaps("kube-system").Get("coredns-cfgmap", metav1.GetOptions{})
			require.NoError(t, err)

			assert.Equal(t, test.expectedCorefile, cfgMap.Data["Corefile"])
		})
	}
}

func TestPrepare_CoreDNSBackup(t *testing.T) {
	tests := []struct {
		desc string

		mockFile string
	}{
		{
			desc: "First time backup of CoreDNS configmap",

			mockFile: "configurecoredns_not_patched.yaml",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clt := k8s.NewClientMock(t, ctx.Done(), test.mockFile, false)

			prep := prepare.NewPrepare(logrus.New(), clt)
			err := prep.ConfigureCoreDNS("titi", "toto")

			assert.NoError(t, err)

			cfgMap, err := clt.KubernetesClient().CoreV1().ConfigMaps("kube-system").Get("coredns-cfgmap", metav1.GetOptions{})
			require.NoError(t, err)

			cfgMapBackup, err := clt.KubernetesClient().CoreV1().ConfigMaps("toto").Get("coredns-cfgmap-backup", metav1.GetOptions{})
			require.NoError(t, err)

			assert.Len(t, cfgMap.Data, 1)
			assert.Equal(t, cfgMap.ObjectMeta.Labels["maesh-backed-up"], "true")
			assert.Len(t, cfgMapBackup.Data, 1)
		})
	}
}

func TestConfigureKubeDNS(t *testing.T) {
	tests := []struct {
		desc string

		mockFile string

		expectedStubDomains string
		expectedErr         bool
	}{
		{
			desc: "First time config of KubeDNS",

			mockFile: "configurekubedns_not_patched.yaml",

			expectedErr:         false,
			expectedStubDomains: `{"maesh":["1.2.3.4"]}`,
		},
		{
			desc: "Already patched",

			mockFile: "configurekubedns_already_patched.yaml",

			expectedErr: false,
		},
		{
			desc: "Missing KubeDNS deployment",

			mockFile: "configurekubedns_missing_deployment.yaml",

			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clt := k8s.NewClientMock(t, ctx.Done(), test.mockFile, false)

			prep := prepare.NewPrepare(logrus.New(), clt)
			err := prep.ConfigureKubeDNS("maesh")
			if test.expectedErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			cfgMap, err := clt.KubernetesClient().CoreV1().ConfigMaps("kube-system").Get("kubedns-cfgmap", metav1.GetOptions{})
			require.NoError(t, err)

			assert.Equal(t, test.expectedStubDomains, cfgMap.Data["stubDomains"])
		})
	}
}
