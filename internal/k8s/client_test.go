package k8s

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	kubeerror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestTranslateNotFoundError(t *testing.T) {
	testCases := []struct {
		desc           string
		err            error
		expectedExists bool
		expectedError  error
	}{
		{
			desc:           "kubernetes not found error",
			err:            kubeerror.NewNotFound(schema.GroupResource{}, "foo"),
			expectedExists: false,
			expectedError:  nil,
		},
		{
			desc:           "nil error",
			err:            nil,
			expectedExists: true,
			expectedError:  nil,
		},
		{
			desc:           "not a kubernetes not found error",
			err:            fmt.Errorf("bar error"),
			expectedExists: false,
			expectedError:  fmt.Errorf("bar error"),
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			exists, err := translateNotFoundError(test.err)
			assert.Equal(t, test.expectedExists, exists)
			assert.Equal(t, test.expectedError, err)
		})
	}
}

func TestParseServiceNamePort(t *testing.T) {
	testCases := []struct {
		desc             string
		given            string
		serviceName      string
		serviceNamespace string
		servicePort      int32
		parseError       bool
	}{
		{
			desc:             "simple parse",
			given:            "foo/bar:80",
			serviceName:      "bar",
			serviceNamespace: "foo",
			servicePort:      80,
		},
		{
			desc:             "empty namespace",
			given:            "bar:80",
			serviceName:      "bar",
			serviceNamespace: "default",
			servicePort:      80,
		},
		{
			desc:             "missing port",
			given:            "foo/bar",
			serviceName:      "",
			serviceNamespace: "",
			servicePort:      0,
			parseError:       true,
		},
		{
			desc:             "unparseable port",
			given:            "foo/bar:%",
			serviceName:      "",
			serviceNamespace: "",
			servicePort:      0,
			parseError:       true,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			name, namespace, port, err := ParseServiceNamePort(test.given)
			assert.Equal(t, test.serviceName, name)
			assert.Equal(t, test.serviceNamespace, namespace)
			assert.Equal(t, test.servicePort, port)
			if test.parseError {
				assert.Error(t, err)
			}
		})
	}
}
