package integration

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"testing"
	"time"

	"github.com/containous/i3o/integration/try"
	"github.com/containous/i3o/internal/k8s"
	"github.com/go-check/check"
	log "github.com/sirupsen/logrus"
	checker "github.com/vdemeester/shakers"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilpointer "k8s.io/utils/pointer"
)

var (
	integration    = flag.Bool("integration", true, "run integration tests")
	kubeConfigPath = "/tmp/k3s-output/kubeconfig.yaml"
	masterURL      = "https://localhost:8443"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

func init() {
	flag.Parse()
	if !*integration {
		log.Info("Integration tests disabled.")
		return
	}

	check.Suite(&StartI3oSuite{})
}

type BaseSuite struct {
	composeProject string
	projectName    string
	dir            string
	try            *try.Try
	clients        *k8s.ClientWrapper
}

func (s *BaseSuite) startk3s(_ *check.C) error {
	var err error
	s.dir, err = os.Getwd()
	if err != nil {
		return err
	}

	if err = os.MkdirAll(path.Join(s.dir, "resources/compose/images"), 0755); err != nil {
		return err
	}
	// Save i3o image in k3s.
	cmd := exec.Command("docker",
		"save", "containous/i3o:latest", "-o", path.Join(s.dir, "resources/compose/images/i3o.tar"))
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	if err != nil {
		return err
	}

	s.composeProject = path.Join(s.dir, "resources/compose/k3s.yaml")
	s.projectName = "integration-test-k3s"

	s.stopComposeProject()

	// Start k3s stack.
	cmd = exec.Command("docker-compose",
		"--file", s.composeProject, "--project-name", s.projectName,
		"up", "-d", "--scale", "node=2")
	cmd.Env = os.Environ()

	output, err = cmd.CombinedOutput()

	fmt.Println(string(output))
	if err != nil {
		return err
	}

	s.clients, err = s.try.WaitClientCreated(masterURL, kubeConfigPath, 30*time.Second)
	if err != nil {
		return err
	}

	s.try = try.NewTry(s.clients)
	return nil
}

func (s *BaseSuite) stopComposeProject() {
	// shutdown and delete compose project
	cmd := exec.Command("docker-compose", "--file", s.composeProject,
		"--project-name", s.projectName,
		"down", "--volumes", "--remove-orphans")
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	if err != nil {
		fmt.Println(err)
	}
}

func (s *BaseSuite) waitForCoreDNSStarted(c *check.C) {
	err := s.try.WaitReadyDeployment("coredns", metav1.NamespaceSystem, 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) waitForI3oControllerStarted(c *check.C) {
	err := s.try.WaitReadyDeployment("i3o-controller", metav1.NamespaceDefault, 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) waitForTiller(c *check.C) {
	err := s.try.WaitReadyDeployment("tiller-deploy", metav1.NamespaceSystem, 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) waitForTools(c *check.C) {
	err := s.try.WaitReadyDeployment("tiny-tools", metav1.NamespaceDefault, 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) startWhoami(c *check.C) {
	// Init helm with the service account created before.
	cmd := exec.Command("kubectl", "apply",
		"-f", path.Join(s.dir, "resources/whoami"))
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	c.Assert(err, checker.IsNil)

	err = s.try.WaitReadyDeployment("whoami", "whoami", 60*time.Second)
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) installHelmI3o(c *check.C) {
	// Delete previous tiller service account.
	err := s.clients.KubeClient.CoreV1().ServiceAccounts(metav1.NamespaceSystem).Delete("tiller", &metav1.DeleteOptions{})
	if err != nil {
		fmt.Println(err)
	}

	// Create tiller service account.
	_, err = s.clients.KubeClient.CoreV1().ServiceAccounts(metav1.NamespaceSystem).Create(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tiller",
			Namespace: metav1.NamespaceSystem,
		},
	})
	c.Assert(err, checker.IsNil)

	// Delete previous tiller cluster role bindings.
	err = s.clients.KubeClient.RbacV1().ClusterRoleBindings().Delete("tiller", &metav1.DeleteOptions{})
	if err != nil {
		fmt.Println(err)
	}

	// Create tiller cluster role bindings.
	_, err = s.clients.KubeClient.RbacV1().ClusterRoleBindings().Create(&rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tiller",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "tiller",
				Namespace: metav1.NamespaceSystem,
			},
		},
	})
	c.Assert(err, checker.IsNil)

	// Init helm with the service account created before.
	cmd := exec.Command("helm", "init",
		"--service-account", "tiller", "--upgrade")
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))
	c.Assert(err, checker.IsNil)

	// Wait for tiller initialized.
	s.waitForTiller(c)

	// Install the helm chart.
	cmd = exec.Command("helm", "install",
		"../helm/chart/i3o", "--values", "resources/values.yaml")
	cmd.Env = os.Environ()

	output, err = cmd.CombinedOutput()

	fmt.Println(string(output))
	c.Assert(err, checker.IsNil)
}

func (s *BaseSuite) installTinyToolsI3o(c *check.C) {
	// Delete previous tiny tools deployment.
	err := s.clients.KubeClient.AppsV1().Deployments(metav1.NamespaceDefault).Delete("tiny-tools", &metav1.DeleteOptions{})
	if err != nil {
		fmt.Println(err)
	}

	// Create new tiny tools deployment.
	_, err = s.clients.KubeClient.AppsV1().Deployments(metav1.NamespaceDefault).Create(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tiny-tools",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilpointer.Int32Ptr(1),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tiny-tools",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "tinyTools",
						Image:   "giantswarm/tiny-tools",
						Command: []string{"sleep", "36000"},
					},
					},
				},
			},
		},
	})
	c.Assert(err, checker.IsNil)

	// Wait for tools to be initialized.
	s.waitForTools(c)
}

func (s *BaseSuite) getToolsPodI3o(c *check.C) *corev1.Pod {
	pod, err := s.clients.KubeClient.CoreV1().Pods(metav1.NamespaceDefault).Get("tiny-tools", metav1.GetOptions{})
	c.Assert(err, checker.IsNil)
	c.Assert(pod, checker.NotNil)

	return pod
}
