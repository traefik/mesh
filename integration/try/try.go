package try

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/containous/i3o/utils"
	"github.com/containous/traefik/pkg/safe"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// CITimeoutMultiplier is the multiplier for all timeout in the CI
	CITimeoutMultiplier = 3
)

// WaitReadyReplica wait until the deployment is ready.
func WaitReadyReplica(clients *utils.ClientWrapper, name string, namespace string, timeout time.Duration) error {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = applyCIMultiplier(timeout)

	if err := backoff.Retry(safe.OperationWithRecover(func() error {
		d, err := clients.KubeClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("unable get the deployment %q in namespace %q: %v", name, namespace, err)
		}

		if d.Status.ReadyReplicas == d.Status.Replicas {
			return errors.New("deployment not ready")
		}
		return nil
	}), ebo); err != nil {
		return fmt.Errorf("unable get the deployment %q in namespace %q: %v", name, namespace, err)
	}

	return nil
}

// WaitClientCreated wait until the file is created.
func WaitClientCreated(url string, kubeConfigPath string, timeout time.Duration) (*utils.ClientWrapper, error) {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = applyCIMultiplier(timeout)

	var clients *utils.ClientWrapper
	var err error
	if err = backoff.Retry(safe.OperationWithRecover(func() error {
		clients, err = utils.BuildClients("https://localhost:6443", kubeConfigPath)
		if err != nil {
			return fmt.Errorf("unable to create clients: %v", err)
		}

		if _, err = clients.KubeClient.ServerVersion(); err != nil {
			return fmt.Errorf("unable to get server version: %v", err)
		}

		return nil
	}), ebo); err != nil {
		return nil, fmt.Errorf("unable to create clients: %v", err)
	}

	return clients, nil
}

func applyCIMultiplier(timeout time.Duration) time.Duration {
	ci := os.Getenv("CI")
	if len(ci) > 0 {
		log.Debug("Apply CI multiplier:", CITimeoutMultiplier)
		return time.Duration(float64(timeout) * CITimeoutMultiplier)
	}
	return timeout
}
