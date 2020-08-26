package k3d

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/containous/maesh/integration/try"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	kubeerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	k3sImage   = "rancher/k3s"
	k3sVersion = "v1.18.6-k3s1"
)

// DockerImage holds the configuration of a Docker image.
type DockerImage struct {
	Name  string
	Local bool
}

// ClusterOptions holds the configuration of the cluster.
type ClusterOptions struct {
	Cmd    []string
	Images []DockerImage
}

// ClusterOptionFunc mutates the given ClusterOptions.
type ClusterOptionFunc func(opts *ClusterOptions)

// WithoutTraefik tells k3d to not start a k3s cluster with Traefik already installed in.
func WithoutTraefik() func(opts *ClusterOptions) {
	return func(opts *ClusterOptions) {
		opts.Cmd = append(opts.Cmd, "--k3s-server-arg", "--no-deploy=traefik")
	}
}

// WithoutCoreDNS tells k3d to not start a k3s cluster with CoreDNS already installed in.
func WithoutCoreDNS() func(opts *ClusterOptions) {
	return func(opts *ClusterOptions) {
		opts.Cmd = append(opts.Cmd, "--k3s-server-arg", "--no-deploy=coredns")
	}
}

// WithImages tells k3d to import the given image. Images which are tagged a local won't be pull locally before being
// imported.
func WithImages(images ...DockerImage) func(opts *ClusterOptions) {
	return func(opts *ClusterOptions) {
		opts.Images = append(opts.Images, images...)
	}
}

// Cluster is a k3s cluster.
type Cluster struct {
	logger     logrus.FieldLogger
	workingDir string
	Client     k8s.Client
	Name       string
}

// NewCluster creates a new k3s cluster using the given configuration.
func NewCluster(logger logrus.FieldLogger, masterURL string, name string, opts ...ClusterOptionFunc) (*Cluster, error) {
	clusterOpts := ClusterOptions{
		Images: []DockerImage{
			{Name: "rancher/coredns-coredns:1.6.3"},
		},
	}

	for _, opt := range opts {
		opt(&clusterOpts)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("unable to get working directory: %w", err)
	}

	if err = pullDockerImages(logger, clusterOpts.Images); err != nil {
		return nil, fmt.Errorf("unable to pull docker images: %w", err)
	}

	if err = createCluster(logger, name, clusterOpts.Cmd); err != nil {
		return nil, fmt.Errorf("unable to create k3s cluster: %d", err)
	}

	if err = importDockerImages(logger, name, clusterOpts.Images); err != nil {
		return nil, fmt.Errorf("unable to import docker images in the cluster: %w", err)
	}

	var client k8s.Client

	client, err = createK8sClient(logger, name, masterURL)
	if err != nil {
		return nil, fmt.Errorf("unable to create kubernetes client: %w", err)
	}

	return &Cluster{
		logger:     logger,
		workingDir: workingDir,
		Client:     client,
		Name:       name,
	}, nil
}

// Stop stops the cluster.
func (c *Cluster) Stop(logger logrus.FieldLogger) error {
	cmd := exec.Command("k3d", "cluster", "delete", c.Name)
	cmd.Env = os.Environ()

	logger.Infof("Stopping k3s cluster %q...", c.Name)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("unable to stop cluster: %w", err)
	}

	return nil
}

// CreateNamespace creates a new namespace.
func (c *Cluster) CreateNamespace(logger logrus.FieldLogger, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}

	logger.Infof("Creating namespace %q...", name)

	_, err := c.Client.KubernetesClient().CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("unable to create namespace %q: %w", name, err)
	}

	return nil
}

// Apply applies the given directory/file on the cluster.
func (c *Cluster) Apply(logger logrus.FieldLogger, resourcesPath string) error {
	opts := []string{
		"apply", "-f", path.Join(c.workingDir, resourcesPath),
	}

	logger.Infof("Applying %q...", resourcesPath)

	cmd := exec.Command("kubectl", opts...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.WithError(err).Errorf("unable to apply resources: %s", string(output))
		return err
	}

	return nil
}

// Delete deletes from the cluster the resources declared in the given directory/file.
func (c *Cluster) Delete(logger logrus.FieldLogger, resourcesPath string) {
	opts := []string{
		"delete", "-f", path.Join(c.workingDir, resourcesPath),
		"--force", "--grace-period=0",
	}

	logger.Infof("Delete %q...", resourcesPath)

	cmd := exec.Command("kubectl", opts...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.WithError(err).Errorf("unable to delete resources: %s", string(output))
	}
}

// WaitReadyDeployment waits for the given deployment to be ready.
func (c *Cluster) WaitReadyDeployment(name, namespace string, timeout time.Duration) error {
	err := try.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		d, err := c.Client.KubernetesClient().AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if kubeerror.IsNotFound(err) {
				return fmt.Errorf("deployment %q has not been yet created", name)
			}

			return fmt.Errorf("unable get deployment %q in namespace %q: %v", name, namespace, err)
		}

		if d.Status.UpdatedReplicas == *(d.Spec.Replicas) &&
			d.Status.Replicas == *(d.Spec.Replicas) &&
			d.Status.AvailableReplicas == *(d.Spec.Replicas) &&
			d.Status.ObservedGeneration >= d.Generation {
			return nil
		}

		return errors.New("deployment not ready")
	}, timeout)

	if err != nil {
		return fmt.Errorf("deployment %q in namespace %q is not ready: %w", name, namespace, err)
	}

	return nil
}

// WaitReadyDaemonSet waits for the given daemonset to be ready.
func (c *Cluster) WaitReadyDaemonSet(name, namespace string, timeout time.Duration) error {
	err := try.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		d, err := c.Client.KubernetesClient().AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if kubeerror.IsNotFound(err) {
				return fmt.Errorf("daemonset %q has not been yet created", name)
			}

			return fmt.Errorf("unable get daemonset %q in namespace %q: %v", name, namespace, err)
		}
		if d.Status.NumberReady == d.Status.DesiredNumberScheduled {
			return nil
		}

		return errors.New("daemonset not ready")
	}, timeout)

	if err != nil {
		return fmt.Errorf("daemonset %q in namespace %q is not ready: %w", name, namespace, err)
	}

	return nil
}

// WaitReadyPod waits for the given pod to be ready.
func (c *Cluster) WaitReadyPod(name, namespace string, timeout time.Duration) error {
	err := try.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		pod, err := c.Client.KubernetesClient().CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if kubeerror.IsNotFound(err) {
				return fmt.Errorf("pod %q has not been yet created", name)
			}

			return fmt.Errorf("unable get pod %q in namespace %q: %v", name, namespace, err)
		}

		if !isPodReady(pod) {
			return errors.New("pod is not ready")
		}

		return nil
	}, timeout)

	if err != nil {
		return fmt.Errorf("pod %q in namespace %q is not ready: %w", name, namespace, err)
	}

	return nil
}

func isPodReady(pod *corev1.Pod) bool {
	var readyCondition *corev1.PodCondition

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			readyCondition = &condition
			break
		}
	}

	return readyCondition != nil && readyCondition.Status == corev1.ConditionTrue
}

func createCluster(logger logrus.FieldLogger, clusterName string, cmdOpts []string) error {
	logger.Infof("Creating k3d cluster %s...", clusterName)

	opts := []string{
		"cluster", "create", clusterName,
		"--no-lb",
		"--api-port", "8443",
		"--agents", "1",
		"--image", fmt.Sprintf("%s:%s", k3sImage, k3sVersion),
		"--timeout", "30s",
	}

	cmd := exec.Command("k3d", append(opts, cmdOpts...)...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.WithError(err).Errorf("unable to create cluster: %s", string(output))
		return err
	}

	return nil
}

func pullDockerImages(logger logrus.FieldLogger, images []DockerImage) error {
	for _, image := range images {
		if image.Local {
			continue
		}

		logger.Infof("Pulling image %q...", image.Name)

		cmd := exec.Command("docker", "pull", image.Name)
		cmd.Env = os.Environ()

		output, err := cmd.CombinedOutput()
		if err != nil {
			logger.WithError(err).Errorf("unable to pull image %q: %s", image.Name, string(output))
			return err
		}
	}

	return nil
}

func importDockerImages(logger logrus.FieldLogger, clusterName string, images []DockerImage) error {
	args := []string{
		"image", "import", "--cluster", clusterName,
	}

	for _, image := range images {
		args = append(args, image.Name)
	}

	logger.Info("Importing images...")

	cmd := exec.Command("k3d", args...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.WithError(err).Errorf("unable to import images: %s", string(output))
		return err
	}

	return nil
}

func createK8sClient(logger logrus.FieldLogger, clusterName, masterURL string) (k8s.Client, error) {
	cmd := exec.Command("k3d", "kubeconfig", "write", clusterName)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve kubeconfig for cluster %q: %w", clusterName, err)
	}

	kubeConfigPath := strings.TrimSuffix(string(output), "\n")

	logger.Info("Creating kubernetes client...")

	var client k8s.Client

	err = try.Retry(func() error {
		client, err = k8s.NewClient(logger, masterURL, kubeConfigPath)
		if err != nil {
			return fmt.Errorf("unable to create clients: %v", err)
		}

		if _, err = client.KubernetesClient().Discovery().ServerVersion(); err != nil {
			return fmt.Errorf("unable to get server version: %v", err)
		}

		return nil
	}, 30*time.Second)

	if err != nil {
		return nil, fmt.Errorf("unable to create kubernetes client: %w", err)
	}

	logger.Infof("Setting new KUBECONFIG path: %q...", kubeConfigPath)

	err = os.Setenv("KUBECONFIG", kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("unable to set KUBECONFIG path: %w", err)
	}

	return client, nil
}
