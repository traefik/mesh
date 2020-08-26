package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/containous/maesh/integration/k3d"
	"github.com/containous/maesh/integration/tool"
	"github.com/containous/maesh/integration/try"
	"github.com/go-check/check"
	"github.com/sirupsen/logrus"
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

	requiredImages := []k3d.DockerImage{
		{Name: "containous/whoami:v1.0.1"},
		{Name: "coredns/coredns:1.3.1"},
		{Name: "coredns/coredns:1.4.0"},
		{Name: "coredns/coredns:1.5.2"},
		{Name: "coredns/coredns:1.6.3"},
		{Name: "coredns/coredns:1.7.0"},
		{Name: "giantswarm/tiny-tools:3.9"},
	}

	s.logger = logrus.New()
	s.cluster, err = k3d.NewCluster(s.logger, masterURL, k3dClusterName,
		k3d.WithoutTraefik(),
		k3d.WithImages(requiredImages...),
	)
	c.Assert(err, checker.IsNil)

	c.Assert(s.cluster.CreateNamespace(s.logger, maeshNamespace), checker.IsNil)
	c.Assert(s.cluster.CreateNamespace(s.logger, testNamespace), checker.IsNil)

	c.Assert(s.cluster.Apply(s.logger, smiCRDs), checker.IsNil)
	c.Assert(s.cluster.Apply(s.logger, "testdata/tool/tool.yaml"), checker.IsNil)
	c.Assert(s.cluster.Apply(s.logger, "testdata/coredns/whoami-shadow-service.yaml"), checker.IsNil)

	c.Assert(s.cluster.WaitReadyPod("tool", testNamespace, 60*time.Second), checker.IsNil)

	s.tool = tool.New(s.logger, "tool", testNamespace)
}

func (s *CoreDNSSuite) TearDownSuite(c *check.C) {
	c.Assert(s.cluster.Stop(s.logger), checker.IsNil)
}

// TestCoreDNSLegacy tests specific version of CoreDNS which did not support the `ready` plugin. It checks that
// the prepare command successfully patches the Corefile or custom Corefile and that we can dig it.
func (s *CoreDNSSuite) TestCoreDNSLegacy(c *check.C) {
	versions := []string{"1.3.1", "1.4.0"}

	c.Assert(s.cluster.Apply(s.logger, "testdata/coredns/coredns-legacy.yaml"), checker.IsNil)
	defer s.cluster.Delete(s.logger, "testdata/coredns/coredns-legacy.yaml")

	c.Assert(s.cluster.WaitReadyDeployment("coredns", metav1.NamespaceSystem, 60*time.Second), checker.IsNil)

	for _, version := range versions {
		s.testCoreDNSVersion(c, version)
	}
}

// TestCoreDNS tests CoreDNS version after 1.4.0 and make sure once patched ".maesh" urls can be resolved
// successfully.
func (s *CoreDNSSuite) TestCoreDNS(c *check.C) {
	versions := []string{"1.5.2", "1.6.3", "1.7.0"}

	c.Assert(s.cluster.Apply(s.logger, "testdata/coredns/coredns.yaml"), checker.IsNil)
	defer s.cluster.Delete(s.logger, "testdata/coredns/coredns.yaml")

	c.Assert(s.cluster.WaitReadyDeployment("coredns", metav1.NamespaceSystem, 60*time.Second), checker.IsNil)

	for _, version := range versions {
		s.testCoreDNSVersion(c, version)
	}
}

func (s *CoreDNSSuite) testCoreDNSVersion(c *check.C, version string) {
	s.logger.Infof("Asserting CoreDNS %s has been patched successfully and can be dug", version)

	err := try.Retry(func() error {
		return s.setCoreDNSVersion(version)
	}, 60*time.Second)
	c.Assert(err, checker.IsNil)

	c.Assert(maeshPrepare(), checker.IsNil)

	// Wait for coreDNS, as the pods will be restarted.
	c.Assert(s.cluster.WaitReadyDeployment("coredns", metav1.NamespaceSystem, 60*time.Second), checker.IsNil)

	err = try.Retry(func() error {
		return s.tool.Dig("whoami.whoami.maesh")
	}, 30*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *CoreDNSSuite) setCoreDNSVersion(version string) error {
	ctx := context.Background()

	// Get current coreDNS deployment.
	deployment, err := s.cluster.Client.KubernetesClient().AppsV1().Deployments(metav1.NamespaceSystem).Get(ctx, "coredns", metav1.GetOptions{})
	if err != nil {
		return err
	}

	newDeployment := deployment.DeepCopy()

	if len(newDeployment.Spec.Template.Spec.Containers) != 1 {
		return fmt.Errorf("expected 1 containers, got %d", len(newDeployment.Spec.Template.Spec.Containers))
	}

	newDeployment.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("coredns/coredns:%s", version)

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, updateErr := s.cluster.Client.KubernetesClient().AppsV1().Deployments(newDeployment.Namespace).Update(context.Background(), newDeployment, metav1.UpdateOptions{})
		return updateErr
	})
	if err != nil {
		return fmt.Errorf("unable to update coredns deployment: %w", err)
	}

	return nil
}
