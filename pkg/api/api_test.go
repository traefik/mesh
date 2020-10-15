package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var localhost = "127.0.0.1"

func TestEnableReadiness(t *testing.T) {
	api := NewAPI(logrus.New(), 9000, localhost, "foo")

	assert.Equal(t, false, api.readiness.Get().(bool))

	api.SetReadiness(true)

	assert.Equal(t, true, api.readiness.Get().(bool))
}

func TestGetReadiness(t *testing.T) {
	testCases := []struct {
		desc               string
		readiness          bool
		expectedStatusCode int
	}{
		{
			desc:               "ready",
			readiness:          true,
			expectedStatusCode: http.StatusOK,
		},
		{
			desc:               "not ready",
			readiness:          false,
			expectedStatusCode: http.StatusInternalServerError,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			api := NewAPI(logrus.New(), 9000, localhost, "foo")

			api.readiness.Set(test.readiness)

			res := httptest.NewRecorder()

			req, err := http.NewRequest(http.MethodGet, "/api/ready", nil)
			require.NoError(t, err)

			api.getReadiness(res, req)

			assert.Equal(t, test.expectedStatusCode, res.Code)
		})
	}
}

func TestGetConfiguration(t *testing.T) {
	api := NewAPI(logrus.New(), 9000, localhost, "foo")

	api.configuration.Set("foo")

	res := httptest.NewRecorder()

	req, err := http.NewRequest(http.MethodGet, "/api/configuration", nil)
	require.NoError(t, err)

	api.getConfiguration(res, req)

	assert.Equal(t, "\"foo\"\n", res.Body.String())
}

func TestGetTopology(t *testing.T) {
	api := NewAPI(logrus.New(), 9000, localhost, "foo")

	api.topology.Set("foo")

	res := httptest.NewRecorder()

	req, err := http.NewRequest(http.MethodGet, "/api/topology", nil)
	require.NoError(t, err)

	api.getTopology(res, req)

	assert.Equal(t, "\"foo\"\n", res.Body.String())
}
