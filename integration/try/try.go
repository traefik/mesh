package try

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	kubeerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

const (
	// CITimeoutMultiplier is the multiplier for all timeout in the CI
	CITimeoutMultiplier = 3
	maxInterval         = 5 * time.Second
)

type timedAction func(timeout time.Duration, operation DoCondition) error

// Try holds try configuration.
type Try struct {
	client k8s.Client
	log    logrus.FieldLogger
}

// NewTry creates a new try.
func NewTry(client k8s.Client) *Try {
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	return &Try{client: client, log: log}
}

// WaitReadyDeployment wait until the deployment is ready.
func (t *Try) WaitReadyDeployment(name string, namespace string, timeout time.Duration) error {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = applyCIMultiplier(timeout)

	if err := backoff.Retry(safe.OperationWithRecover(func() error {
		d, err := t.client.KubernetesClient().AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			if kubeerror.IsNotFound(err) {
				return fmt.Errorf("deployment %q has not been yet created", name)
			}

			return fmt.Errorf("unable get the deployment %q in namespace %q: %v", name, namespace, err)
		}
		if d.Status.Replicas == 0 {
			return fmt.Errorf("deployment %q has no replicas", name)
		}

		if d.Status.UnavailableReplicas > 0 {
			return fmt.Errorf("deployment %q has unavailable replicas", name)
		}

		if d.Status.ReadyReplicas == d.Status.Replicas && d.Status.ReadyReplicas == d.Status.AvailableReplicas {
			return nil
		}
		return errors.New("deployment not ready")
	}), ebo); err != nil {
		return fmt.Errorf("unable get the deployment %q in namespace %q: %v", name, namespace, err)
	}

	return nil
}

// WaitReadyDaemonset wait until the daemonset is ready.
func (t *Try) WaitReadyDaemonset(name string, namespace string, timeout time.Duration) error {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = applyCIMultiplier(timeout)

	if err := backoff.Retry(safe.OperationWithRecover(func() error {
		d, err := t.client.KubernetesClient().AppsV1().DaemonSets(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			if kubeerror.IsNotFound(err) {
				return fmt.Errorf("daemonset %q has not been yet created", name)
			}

			return fmt.Errorf("unable get the daemonset %q in namespace %q: %v", name, namespace, err)
		}
		if d.Status.NumberReady == d.Status.DesiredNumberScheduled {
			return nil
		}
		return errors.New("daemonset not ready")
	}), ebo); err != nil {
		return fmt.Errorf("unable get the daemonset %q in namespace %q: %v", name, namespace, err)
	}

	return nil
}

// WaitUpdateDeployment waits until the deployment is successfully updated and ready.
func (t *Try) WaitUpdateDeployment(deployment *appsv1.Deployment, timeout time.Duration) error {
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := t.client.KubernetesClient().AppsV1().Deployments(deployment.Namespace).Update(deployment)
		return err
	})

	if retryErr != nil {
		return fmt.Errorf("unable to update deployment %q: %v", deployment.Name, retryErr)
	}

	return t.WaitReadyDeployment(deployment.Name, deployment.Namespace, timeout)
}

// WaitDeleteDeployment wait until the deployment is delete.
func (t *Try) WaitDeleteDeployment(name string, namespace string, timeout time.Duration) error {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = applyCIMultiplier(timeout)

	if err := backoff.Retry(safe.OperationWithRecover(func() error {
		_, err := t.client.KubernetesClient().AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			if kubeerror.IsNotFound(err) {
				return nil
			}

			return fmt.Errorf("unable get the deployment %q in namespace %q: %v", name, namespace, err)
		}

		return fmt.Errorf("deployment %q exist", name)
	}), ebo); err != nil {
		return fmt.Errorf("unable get the deployment %q in namespace %q: %v", name, namespace, err)
	}

	return nil
}

// WaitPodIPAssigned wait until the pod is assigned an IP.
func (t *Try) WaitPodIPAssigned(name string, namespace string, timeout time.Duration) error {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = applyCIMultiplier(timeout)

	if err := backoff.Retry(safe.OperationWithRecover(func() error {
		pod, err := t.client.KubernetesClient().CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("unable get the pod %q in namespace %q: %v", name, namespace, err)
		}

		// If the pod IP is not empty, log and return.
		if pod.Status.PodIP != "" {
			// IP is assigned
			fmt.Printf("Pod %q has IP: %s\n", name, pod.Status.PodIP)
			return nil
		}

		return errors.New("pod does not have an IP assigned")
	}), ebo); err != nil {
		return fmt.Errorf("unable get the pod IP for pod %s: %v", name, err)
	}

	return nil
}

// WaitCommandExecute wait until the command is executed.
func (t *Try) WaitCommandExecute(command string, argSlice []string, expected string, timeout time.Duration) error {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = applyCIMultiplier(timeout)

	var output []byte

	if err := backoff.Retry(safe.OperationWithRecover(func() error {
		cmd := exec.Command(command, argSlice...)
		cmd.Env = os.Environ()
		var errOpt error
		output, errOpt = cmd.CombinedOutput()
		if errOpt != nil {
			return fmt.Errorf("unable execute command %s %s - output %s: \n%v", command, strings.Join(argSlice, " "), output, errOpt)
		}

		if !strings.Contains(string(output), expected) {
			return fmt.Errorf("output %s does not contain %s", string(output), expected)
		}

		t.log.Debug(string(output))

		return nil
	}), ebo); err != nil {
		return fmt.Errorf("unable execute command %s %s: \n%v", command, strings.Join(argSlice, " "), err)
	}

	return nil
}

// WaitCommandExecuteReturn wait until the command is executed.
func (t *Try) WaitCommandExecuteReturn(command string, argSlice []string, timeout time.Duration) (string, error) {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = applyCIMultiplier(timeout)

	var output []byte

	if err := backoff.Retry(safe.OperationWithRecover(func() error {
		cmd := exec.Command(command, argSlice...)
		cmd.Env = os.Environ()
		var errOpt error
		output, errOpt = cmd.CombinedOutput()
		if errOpt != nil {
			return fmt.Errorf("unable execute command %s %s - output %s: \n%v", command, strings.Join(argSlice, " "), output, errOpt)
		}

		return nil
	}), ebo); err != nil {
		return "", fmt.Errorf("unable execute command %s %s: \n%v", command, strings.Join(argSlice, " "), err)
	}

	return string(output), nil
}

// Retry runs a function over and over until it doesn't return an error or the given timeout duration is reached.
func (t *Try) Retry(f func() error, timeout time.Duration) error {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = applyCIMultiplier(timeout)

	if err := backoff.Retry(safe.OperationWithRecover(f), ebo); err != nil {
		return fmt.Errorf("unable execute function: %v", err)
	}

	return nil
}

// WaitDeleteNamespace wait until the namespace is delete.
func (t *Try) WaitDeleteNamespace(name string, timeout time.Duration) error {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = applyCIMultiplier(timeout)

	if err := backoff.Retry(safe.OperationWithRecover(func() error {
		_, err := t.client.KubernetesClient().CoreV1().Namespaces().Get(name, metav1.GetOptions{})
		if err != nil {
			if kubeerror.IsNotFound(err) {
				return nil
			}

			return fmt.Errorf("unable get the namesapce %q: %v", name, err)
		}

		return fmt.Errorf("namesapce %q exist", name)
	}), ebo); err != nil {
		return fmt.Errorf("unable get the namesapce %q: %v", name, err)
	}

	return nil
}

// WaitClientCreated wait until the file is created.
func (t *Try) WaitClientCreated(url string, kubeConfigPath string, timeout time.Duration) (k8s.Client, error) {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = applyCIMultiplier(timeout)

	var (
		clients k8s.Client
		err     error
	)

	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	if err = backoff.Retry(safe.OperationWithRecover(func() error {
		clients, err = k8s.NewClient(log, url, kubeConfigPath)
		if err != nil {
			return fmt.Errorf("unable to create clients: %v", err)
		}

		if _, err = clients.KubernetesClient().Discovery().ServerVersion(); err != nil {
			return fmt.Errorf("unable to get server version: %v", err)
		}

		return nil
	}), ebo); err != nil {
		return nil, fmt.Errorf("unable to create clients: %v", err)
	}

	return clients, nil
}

// GetRequest is like Do, but runs a request against the given URL and applies
// the condition on the response.
// ResponseCondition may be nil, in which case only the request against the URL must
// succeed.
func GetRequest(url string, timeout time.Duration, conditions ...ResponseCondition) error {
	resp, err := doTryGet(url, timeout, nil, conditions...)

	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	return err
}

func doTryGet(url string, timeout time.Duration, transport http.RoundTripper, conditions ...ResponseCondition) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	return doTryRequest(req, timeout, transport, conditions...)
}

func doTryRequest(request *http.Request, timeout time.Duration, transport http.RoundTripper, conditions ...ResponseCondition) (*http.Response, error) {
	return doRequest(Do, timeout, request, transport, conditions...)
}

func doRequest(action timedAction, timeout time.Duration, request *http.Request, transport http.RoundTripper, conditions ...ResponseCondition) (*http.Response, error) {
	var resp *http.Response

	return resp, action(timeout, func() error {
		var err error
		client := http.DefaultClient
		if transport != nil {
			client.Transport = transport
		}

		resp, err = client.Do(request)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		for _, condition := range conditions {
			if err := condition(resp); err != nil {
				return err
			}
		}

		return nil
	})
}

// Do repeatedly executes an operation until no error condition occurs or the
// given timeout is reached, whatever comes first.
func Do(timeout time.Duration, operation DoCondition) error {
	if timeout <= 0 {
		panic("timeout must be larger than zero")
	}

	interval := time.Duration(math.Ceil(float64(timeout) / 15.0))
	if interval > maxInterval {
		interval = maxInterval
	}

	timeout = applyCIMultiplier(timeout)

	var err error
	if err = operation(); err == nil {
		fmt.Println("+")
		return nil
	}

	fmt.Print("*")

	stopTimer := time.NewTimer(timeout)
	defer stopTimer.Stop()

	retryTick := time.NewTicker(interval)
	defer retryTick.Stop()

	for {
		select {
		case <-stopTimer.C:
			fmt.Println("-")
			return fmt.Errorf("try operation failed: %s", err)
		case <-retryTick.C:
			fmt.Print("*")

			if err = operation(); err == nil {
				fmt.Println("+")
				return err
			}
		}
	}
}

func applyCIMultiplier(timeout time.Duration) time.Duration {
	if os.Getenv("CI") == "" {
		return timeout
	}

	ciTimeoutMultiplier := getCITimeoutMultiplier()
	logrus.Debug("Apply CI multiplier:", ciTimeoutMultiplier)

	return time.Duration(float64(timeout) * ciTimeoutMultiplier)
}

func getCITimeoutMultiplier() float64 {
	ciTimeoutMultiplier := os.Getenv("CI_TIMEOUT_MULTIPLIER")
	if ciTimeoutMultiplier == "" {
		return CITimeoutMultiplier
	}

	multiplier, err := strconv.ParseFloat(ciTimeoutMultiplier, 64)
	if err != nil {
		return CITimeoutMultiplier
	}

	return multiplier
}
