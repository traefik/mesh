package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/containous/traefik/v2/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
)

func TestEnableReadiness(t *testing.T) {
	config := safe.Safe{}
	api := NewAPI(9000, &config, nil)

	assert.Equal(t, false, api.readiness)

	api.EnableReadiness()

	assert.Equal(t, true, api.readiness)
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
			config := safe.Safe{}
			api := NewAPI(9000, &config, nil)
			api.readiness = test.readiness

			res := httptest.NewRecorder()
			req := testhelpers.MustNewRequest(http.MethodGet, "/api/status/readiness", nil)

			api.getReadiness(res, req)

			assert.Equal(t, test.expectedStatusCode, res.Code)
		})
	}
}

func TestGetCurrentConfiguration(t *testing.T) {
	config := safe.Safe{}
	api := NewAPI(9000, &config, nil)

	config.Set("foo")

	res := httptest.NewRecorder()
	req := testhelpers.MustNewRequest(http.MethodGet, "/api/configuration/current", nil)

	api.getCurrentConfiguration(res, req)

	assert.Equal(t, "\"foo\"\n", res.Body.String())
}
