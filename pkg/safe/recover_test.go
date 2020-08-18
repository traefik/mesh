package safe

import (
	"fmt"
	"testing"

	"github.com/cenkalti/backoff/v4"
)

func TestOperationWithRecover(t *testing.T) {
	operation := func() error {
		return nil
	}

	err := backoff.Retry(OperationWithRecover(operation), &backoff.StopBackOff{})
	if err != nil {
		t.Fatalf("Error in OperationWithRecover: %s", err)
	}
}

func TestOperationWithRecoverPanic(t *testing.T) {
	operation := func() error {
		panic("BOOM")
	}

	err := backoff.Retry(OperationWithRecover(operation), &backoff.StopBackOff{})
	if err == nil {
		t.Fatalf("Error in OperationWithRecover: %s", err)
	}
}

func TestOperationWithRecoverError(t *testing.T) {
	operation := func() error {
		return fmt.Errorf("ERROR")
	}

	err := backoff.Retry(OperationWithRecover(operation), &backoff.StopBackOff{})
	if err == nil {
		t.Fatalf("Error in OperationWithRecover: %s", err)
	}
}
