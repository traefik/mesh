package deployer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v3"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
)

// REST is a deployer that pushes dynamic configuration to maesh-nodes using traefik's rest provider.
type REST struct {
	client *http.Client
	logger logrus.FieldLogger
}

func NewREST(logger logrus.FieldLogger) *REST {
	return &REST{
		client: &http.Client{Timeout: time.Second},
		logger: logger.WithField("module", "deployer"),
	}
}

func (pr *REST) Deploy(pods []*corev1.Pod, cfg *dynamic.Configuration) error {
	rawCfg, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("unable to marshal configuration: %v", err)
	}

	var errg errgroup.Group

	for _, p := range pods {
		pod := p

		errg.Go(func() error {
			b := backoff.NewExponentialBackOff()
			b.MaxElapsedTime = 15 * time.Second

			op := func() error {
				if err := pr.deployToPod(pod, rawCfg); err != nil {
					pr.logger.Errorf(
						"unable to deploy dynamic configuration to pod %q: %v",
						pod.GetName(),
						err,
					)

					return err
				}

				return nil
			}

			return backoff.Retry(safe.OperationWithRecover(op), b)
		})
	}

	if err := errg.Wait(); err != nil {
		return fmt.Errorf("one or more deployment has failed: %w", err)
	}

	return nil
}

func (pr *REST) deployToPod(pod *corev1.Pod, cfg []byte) error {
	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("http://%s:8080/api/providers/rest", pod.Status.PodIP),
		bytes.NewBuffer(cfg),
	)
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	resp, err := pr.client.Do(req)
	if err != nil {
		return fmt.Errorf("unable to deploy configuration: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("node answered HTTP status %d != 200", resp.StatusCode)
	}

	pr.logger.Infof("Successfully updated pod %q configuration", pod.GetName())

	return nil
}
