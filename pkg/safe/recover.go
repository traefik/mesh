package safe

import (
	"fmt"
	"runtime/debug"

	"github.com/cenkalti/backoff/v4"
	"github.com/sirupsen/logrus"
)

// OperationWithRecover wrap a backoff operation in a Recover.
func OperationWithRecover(operation backoff.Operation) backoff.Operation {
	return func() (err error) {
		defer func() {
			if res := recover(); res != nil {
				logger := logrus.StandardLogger()

				logger.Errorf("Error in Go routine: %s", res)
				logger.Errorf("Stack: %s", debug.Stack())

				err = fmt.Errorf("panic in operation: %w", err)
			}
		}()

		return operation()
	}
}
