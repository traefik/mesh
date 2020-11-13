package try

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/traefik/mesh/v2/pkg/safe"
)

// CITimeoutMultiplier is the multiplier for all timeout in the CI.
const CITimeoutMultiplier = 3

// Retry runs a function over and over until it doesn't return an error or the given timeout duration is reached.
func Retry(f func() error, timeout time.Duration) error {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = applyCIMultiplier(timeout)

	if err := backoff.Retry(safe.OperationWithRecover(f), ebo); err != nil {
		return fmt.Errorf("unable execute function: %w", err)
	}

	return nil
}

func applyCIMultiplier(timeout time.Duration) time.Duration {
	if os.Getenv("CI") == "" {
		return timeout
	}

	ciTimeoutMultiplier := getCITimeoutMultiplier()

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
