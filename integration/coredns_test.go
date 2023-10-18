package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/go-check/check"
	"github.com/sirupsen/logrus"
	"github.com/traefik/mesh/integration/k3d"
	"github.com/traefik/mesh/integration/tool"
	"github.com/traefik/mesh/integration/try"
	checker "github.com/vdemeester/shakers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

// CoreDNSSuite.
type CoreDNSSuite struct {
	logger  logrus.FieldLogger
	cluster *k3d.Cluster
	tool    *tool.Tool
}

func (s *CoreDNSSuite) SetUpSuite(c *check.C) {
	var err error

	s.logger = logrus.New()

	opts := []k3d.ClusterOptionFunc{k3d.WithoutTraefik()}
	if !*disableImport {
		opts = append(opts, k3d.WithImages(
			k3d.DockerImage{Name: "traefik/whoami:v1.10.1"},
			k3d.DockerImage{Name: "coredns/coredns:1.3.1"},
			k3d.DockerImage{Name: "coredns/coredns:1.4.0"},
			k3d.DockerImage{Name: "coredns/coredns:1.5.2"},
			k3d.DockerImage{Name: "coredns/coredns:1.6.3"},
			k3d.DockerImage{Name: "coredns/coredns:1.7.0"},
			k3d.DockerImage{Name: "coredns/coredns:1.8.0"},
			k3d.DockerImage{Name: "coredns/coredns:1.9.0"},
			k3d.DockerImage{Name: "giantswarm/tiny-tools:3.12"},
		))
	}

	s.cluster, err = k3d.NewCluster(s.logger, masterURL, k3dClusterName, opts...)
	c.Assert(err, checker.IsNil)

	c.Assert(s.cluster.CreateNamespace(s.logger, traefikMeshNamespace), checker.IsNil)
	c.Assert(s.cluster.CreateNamespace(s.logger, testNamespace), checker.IsNil)

	c.Assert(s.cluster.Apply(s.logger, smiCRDs), checker.IsNil)
	c.Assert(s.cluster.Apply(s.logger, "testdata/tool/tool.yaml"), checker.IsNil)
	c.Assert(s.cluster.Apply(s.logger, "testdata/coredns/whoami-shadow-service.yaml"), checker.IsNil)

	c.Assert(s.cluster.WaitReadyPod("tool", testNamespace, 60*time.Second), checker.IsNil)

	s.tool = tool.New(s.logger, "tool", testNamespace)
}

func (s *CoreDNSSuite) TearDownSuite(c *check.C) {
	if s.cluster != nil {
		c.Assert(s.cluster.Stop(s.logger), checker.IsNil)
	}
}

func (s *CoreDNSSuite) TestCoreDNS(c *check.C) {
	versions := []string{"1.5.2", "1.6.3", "1.7.0"}

	for _, version := range versions {
		c.Assert(s.resetCoreDNSCorefile(true), checker.IsNil)

		s.testCoreDNSVersion(c, version)
	}

	// Test specific versions of CoreDNS which did not support the `ready` plugin.
	c.Assert(s.removeCoreDNSReadinessProbe(), checker.IsNil)

	versions = []string{"1.3.1", "1.4.0"}

	for _, version := range versions {
		c.Assert(s.resetCoreDNSCorefile(false), checker.IsNil)

		s.testCoreDNSVersion(c, version)
	}
}

func (s *CoreDNSSuite) testCoreDNSVersion(c *check.C, version string) {
	s.logger.Infof("Asserting CoreDNS %s has been patched successfully and can be dug", version)

	c.Assert(s.setCoreDNSVersion(version), checker.IsNil)
	c.Assert(s.cluster.WaitReadyDeployment("coredns", metav1.NamespaceSystem, 60*time.Second), checker.IsNil)

	c.Assert(traefikMeshPrepare(), checker.IsNil)
	c.Assert(s.cluster.WaitReadyDeployment("coredns", metav1.NamespaceSystem, 60*time.Second), checker.IsNil)

	err := try.Retry(func() error {
		return s.tool.Dig("whoami.whoami.maesh")
	}, 30*time.Second)
	c.Assert(err, checker.IsNil)

	err = try.Retry(func() error {
		return s.tool.Dig("whoami.whoami.traefik.mesh")
	}, 30*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *CoreDNSSuite) setCoreDNSVersion(version string) error {
	ctx := context.Background()

	s.logger.Debugf("Updating CoreDNS version to %q...", version)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get current coreDNS deployment.
		deployment, err := s.cluster.Client.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "coredns", metav1.GetOptions{})
		if err != nil {
			return err
		}

		deployment.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("coredns/coredns:%s", version)

		_, err = s.cluster.Client.KubernetesClient().AppsV1().Deployments(deployment.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		return err
	})
}

func (s *CoreDNSSuite) removeCoreDNSReadinessProbe() error {
	ctx := context.Background()

	s.logger.Debug("Removing CoreDNS readiness probe...")

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deployment, err := s.cluster.Client.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "coredns", metav1.GetOptions{})
		if err != nil {
			return err
		}

		deployment.Spec.Template.Spec.Containers[0].ReadinessProbe = nil

		_, err = s.cluster.Client.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Update(ctx, deployment, metav1.UpdateOptions{})
		return err
	})
}

func (s *CoreDNSSuite) resetCoreDNSCorefile(ready bool) error {
	ctx := context.Background()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		configmap, err := s.cluster.Client.KubernetesClient().CoreV1().ConfigMaps(metav1.NamespaceSystem).Get(ctx, "coredns", metav1.GetOptions{})
		if err != nil {
			return err
		}

		var readyPlugin string

		if ready {
			readyPlugin = "ready"
		}

		configmap.Data["Corefile"] = fmt.Sprintf(`.:53 {
	errors
	health
	%s
	kubernetes cluster.local in-addr.arpa ip6.arpa {
		pods insecure
		fallthrough in-addr.arpa ip6.arpa
	}
	hosts /etc/coredns/NodeHosts {
		reload 1s
		fallthrough
	}
	prometheus :9153
	forward . /etc/resolv.conf
	cache 30
	loop
	reload
	loadbalance
}
`, readyPlugin)

		_, err = s.cluster.Client.KubernetesClient().CoreV1().ConfigMaps(metav1.NamespaceSystem).Update(ctx, configmap, metav1.UpdateOptions{})
		return err
	})
}
