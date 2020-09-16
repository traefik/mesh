package integration

import (
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-check/check"
	"github.com/sirupsen/logrus"
	"github.com/traefik/mesh/integration/k3d"
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
		{Name: "traefik/mesh:latest", Local: true},
		{Name: "traefik:v2.3"},
	}

	s.logger = logrus.New()
	s.cluster, err = k3d.NewCluster(s.logger, masterURL, k3dClusterName,
		k3d.WithoutTraefik(),
		k3d.WithImages(requiredImages...),
	)
	c.Assert(err, checker.IsNil)

	c.Assert(s.cluster.CreateNamespace(s.logger, traefikMeshNamespace), checker.IsNil)
}

func (s *HelmSuite) TearDownSuite(c *check.C) {
	if s.cluster != nil {
		c.Assert(s.cluster.Stop(s.logger), checker.IsNil)
	}
}

func (s *HelmSuite) TestACLDisabled(c *check.C) {
	s.installHelmTraefikMesh(c, false, false)
	defer s.uninstallHelmTraefikMesh(c)

	c.Assert(s.cluster.WaitReadyDeployment("traefik-mesh-controller", traefikMeshNamespace, 60*time.Second), checker.IsNil)
	c.Assert(s.cluster.WaitReadyDaemonSet("traefik-mesh-proxy", traefikMeshNamespace, 60*time.Second), checker.IsNil)
}

func (s *HelmSuite) TestACLEnabled(c *check.C) {
	s.installHelmTraefikMesh(c, true, false)
	defer s.uninstallHelmTraefikMesh(c)

	c.Assert(s.cluster.WaitReadyDeployment("traefik-mesh-controller", traefikMeshNamespace, 60*time.Second), checker.IsNil)
	c.Assert(s.cluster.WaitReadyDaemonSet("traefik-mesh-proxy", traefikMeshNamespace, 60*time.Second), checker.IsNil)
}

func (s *HelmSuite) TestKubeDNSEnabled(c *check.C) {
	s.installHelmTraefikMesh(c, false, true)
	defer s.uninstallHelmTraefikMesh(c)

	c.Assert(s.cluster.WaitReadyDeployment("traefik-mesh-controller", traefikMeshNamespace, 60*time.Second), checker.IsNil)
	c.Assert(s.cluster.WaitReadyDaemonSet("traefik-mesh-proxy", traefikMeshNamespace, 60*time.Second), checker.IsNil)
}

func (s *HelmSuite) installHelmTraefikMesh(c *check.C, acl bool, kubeDNS bool) {
	s.logger.Info("Installing Traefik Mesh via helm...")

	args := []string{
		"install", "powpow", "../helm/chart/mesh",
		"--values", "testdata/traefik-mesh/values.yaml",
		"--namespace", traefikMeshNamespace,
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

func (s *HelmSuite) uninstallHelmTraefikMesh(c *check.C) {
	s.logger.Info("Uninstalling Traefik Mesh via helm...")

	args := []string{
		"uninstall", "powpow",
		"--namespace", traefikMeshNamespace,
	}

	cmd := exec.Command("helm", args...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		c.Errorf("unable execute command 'helm %s' - output %s: %w", strings.Join(args, " "), output, err)
		c.Fail()
	}
}
