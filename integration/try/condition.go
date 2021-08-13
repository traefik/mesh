package try

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// StringCondition is a retry condition function.
// It receives a string, and returns an error
// if the string failed the condition.
type StringCondition func(string) error

// StringContains returns a retry condition function.
// The condition returns an error if the string does not contain the given values.
func StringContains(values ...string) StringCondition {
	return func(res string) error {
		for _, value := range values {
			if !strings.Contains(res, value) {
				return fmt.Errorf("could not find %q in %q", value, res)
			}
		}

		return nil
	}
}

// ResponseCondition is a retry condition function.
// It receives a response, and returns an error
// if the response failed the condition.
type ResponseCondition func(*http.Response) error

// BodyContains returns a retry condition function.
// The condition returns an error if the request body does not contain all the given
// strings.
func BodyContains(values ...string) ResponseCondition {
	return func(res *http.Response) error {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		for _, value := range values {
			if !strings.Contains(string(body), value) {
				return fmt.Errorf("could not find '%s' in body '%s'", value, string(body))
			}
		}

		return nil
	}
}

// StatusCodeIs returns a retry condition function.
// The condition returns an error if the given response's status code is not the
// given HTTP status code.
func StatusCodeIs(status int) ResponseCondition {
	return func(res *http.Response) error {
		if res.StatusCode != status {
			return fmt.Errorf("got status code %d, wanted %d", res.StatusCode, status)
		}

		return nil
	}
}
