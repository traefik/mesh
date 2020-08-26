package integration

import (
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/containous/maesh/integration/k3d"
	"github.com/go-check/check"
	"github.com/sirupsen/logrus"
	checker "github.com/vdemeester/shakers"
)

// HelmSuite.
type HelmSuite struct {
	logger  logrus.FieldLogger
	cluster *k3d.Cluster
}

func (s *HelmSuite) SetUpSuite(c *check.C) {
	var err error

	requiredImages := []k3d.DockerImage{
		{Name: "containous/maesh:latest", Local: true},
		{Name: "traefik:v2.3"},
	}

	s.logger = logrus.New()
	s.cluster, err = k3d.NewCluster(s.logger, masterURL, k3dClusterName,
		k3d.WithoutTraefik(),
		k3d.WithImages(requiredImages...),
	)
	c.Assert(err, checker.IsNil)

	c.Assert(s.cluster.CreateNamespace(s.logger, maeshNamespace), checker.IsNil)
}

func (s *HelmSuite) TearDownSuite(c *check.C) {
	c.Assert(s.cluster.Stop(s.logger), checker.IsNil)
}

func (s *HelmSuite) TestACLDisabled(c *check.C) {
	s.installHelmMaesh(c, false, false)
	defer s.uninstallHelmMaesh(c)

	c.Assert(s.cluster.WaitReadyDeployment("maesh-controller", maeshNamespace, 60*time.Second), checker.IsNil)
	c.Assert(s.cluster.WaitReadyDaemonSet("maesh-mesh", maeshNamespace, 60*time.Second), checker.IsNil)
}

func (s *HelmSuite) TestACLEnabled(c *check.C) {
	s.installHelmMaesh(c, true, false)
	defer s.uninstallHelmMaesh(c)

	c.Assert(s.cluster.WaitReadyDeployment("maesh-controller", maeshNamespace, 60*time.Second), checker.IsNil)
	c.Assert(s.cluster.WaitReadyDaemonSet("maesh-mesh", maeshNamespace, 60*time.Second), checker.IsNil)
}

func (s *HelmSuite) TestKubeDNSEnabled(c *check.C) {
	s.installHelmMaesh(c, false, true)
	defer s.uninstallHelmMaesh(c)

	c.Assert(s.cluster.WaitReadyDeployment("maesh-controller", maeshNamespace, 60*time.Second), checker.IsNil)
	c.Assert(s.cluster.WaitReadyDaemonSet("maesh-mesh", maeshNamespace, 60*time.Second), checker.IsNil)
}

func (s *HelmSuite) installHelmMaesh(c *check.C, acl bool, kubeDNS bool) {
	s.logger.Info("Installing Maesh via helm...")

	args := []string{
		"install", "powpow", "../helm/chart/maesh",
		"--values", "testdata/maesh/values.yaml",
		"--namespace", maeshNamespace,
	}

	if kubeDNS {
		args = append(args, "--set", "kubedns=true")
	}

	if acl {
		args = append(args, "--set", "acl=true")
	}

	cmd := exec.Command("helm", args...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		c.Errorf("unable execute command 'helm %s' - output %s: %w", strings.Join(args, " "), output, err)
		c.Fail()
	}
}

func (s *HelmSuite) uninstallHelmMaesh(c *check.C) {
	s.logger.Info("Uninstalling Maesh via helm...")

	args := []string{
		"uninstall", "powpow",
		"--namespace", maeshNamespace,
	}

	cmd := exec.Command("helm", args...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		c.Errorf("unable execute command 'helm %s' - output %s: %w", strings.Join(args, " "), output, err)
		c.Fail()
	}
}
