package tool

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/traefik/mesh/v2/integration/try"
)

// Tool is pod capable of running operations from within the cluster.
type Tool struct {
	logger         logrus.FieldLogger
	name           string
	namespace      string
	kubectlTimeout time.Duration
}

// New creates a new Tool.
func New(logger logrus.FieldLogger, name, namespace string) *Tool {
	return &Tool{
		name:           name,
		namespace:      namespace,
		logger:         logger,
		kubectlTimeout: 3 * time.Second,
	}
}

// Dig digs the given url and make sure there is an A record.
func (t *Tool) Dig(url string) error {
	output, err := t.exec([]string{"dig", url, "+short", "+timeout=3"})
	if err != nil {
		t.logger.WithError(err).Debug("Dig command has failed")
		return err
	}

	IP := net.ParseIP(strings.TrimSpace(string(output)))
	if IP == nil {
		return fmt.Errorf("could not parse an IP from dig: %s", string(output))
	}

	t.logger.Debugf("Dig %q: %s", url, IP.String())

	return nil
}

// Curl curls the given url and checks if it matches the given conditions.
func (t *Tool) Curl(url string, headers map[string]string, conditions ...try.ResponseCondition) error {
	args := []string{"curl", "-s", "-D-", "-m", "2"}

	for key, value := range headers {
		args = append(args, "-H", key+": "+value)
	}

	output, err := t.exec(append(args, url))
	if err != nil {
		t.logger.WithError(err).Debug("Curl command has failed")
		return err
	}

	reader := bufio.NewReader(bytes.NewReader(output))

	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.logger.WithError(err).Debug("Curl output is not an HTTP response")
		return err
	}

	defer resp.Body.Close()

	for _, condition := range conditions {
		if err := condition(resp); err != nil {
			t.logger.Debugf("Curl condition did not match: %v", err)

			return err
		}
	}

	return nil
}

// Netcat netcats the given url on the given port and checks if the given conditions are fulfilled.
func (t *Tool) Netcat(url string, port int, udp bool, conditions ...try.StringCondition) error {
	var cmd string

	if udp {
		cmd = fmt.Sprintf("echo 'WHO' | nc -u -w 1 %s %d", url, port)
	} else {
		cmd = fmt.Sprintf("echo 'WHO' | nc -q 0 %s %d", url, port)
	}

	output, err := t.exec([]string{"ash", "-c", cmd})
	if err != nil {
		t.logger.WithError(err).Debug("Netcat command has failed")
		return err
	}

	for _, condition := range conditions {
		if err := condition(string(output)); err != nil {
			t.logger.Debugf("Netcat condition did not match: %v", err)

			return err
		}
	}

	return nil
}

func (t *Tool) exec(args []string) ([]byte, error) {
	args = append([]string{
		"exec", "-i", t.name,
		"--request-timeout=3s",
		"-n", t.namespace,
		"--",
	}, args...)

	cmd := exec.Command("kubectl", args...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return []byte{}, fmt.Errorf("unable execute command 'kubectl %s' - output %s: %w", strings.Join(args, " "), output, err)
	}

	return output, nil
}
