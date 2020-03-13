package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/containous/maesh/pkg/deploylog"
	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/containous/traefik/v2/pkg/testhelpers"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var (
	localhost = "127.0.0.1"
)

func TestEnableReadiness(t *testing.T) {
	config := safe.Safe{}
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	api := NewAPI(log, 9000, localhost, &config, nil, nil, "foo")

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
			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			api := NewAPI(log, 9000, localhost, &config, nil, nil, "foo")
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
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	api := NewAPI(log, 9000, localhost, &config, nil, nil, "foo")

	config.Set("foo")

	res := httptest.NewRecorder()
	req := testhelpers.MustNewRequest(http.MethodGet, "/api/configuration/current", nil)

	api.getCurrentConfiguration(res, req)

	assert.Equal(t, "\"foo\"\n", res.Body.String())
}

func TestGetDeployLog(t *testing.T) {
	config := safe.Safe{}
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	deploylog := deploylog.NewDeployLog(log, 1000)
	api := NewAPI(log, 9000, localhost, &config, deploylog, nil, "foo")
	currentTime := time.Now()
	deploylog.LogDeploy(currentTime, "foo", "bar", true, "blabla")

	data, err := currentTime.MarshalJSON()
	assert.NoError(t, err)

	currentTimeString := string(data)
	expected := fmt.Sprintf("[{\"TimeStamp\":%s,\"PodName\":\"foo\",\"PodIP\":\"bar\",\"DeploySuccessful\":true,\"Reason\":\"blabla\"}]", currentTimeString)

	res := httptest.NewRecorder()
	req := testhelpers.MustNewRequest(http.MethodGet, "/api/configuration/current", nil)

	api.getDeployLog(res, req)
	assert.Equal(t, expected, res.Body.String())
	assert.Equal(t, http.StatusOK, res.Code)
}

func TestGetMeshNodes(t *testing.T) {
	testCases := []struct {
		desc               string
		mockFile           string
		expectedBody       string
		expectedStatusCode int
		podError           bool
	}{
		{
			desc:               "empty mesh node list",
			mockFile:           "getmeshnodes_empty.yaml",
			expectedBody:       "[]\n",
			expectedStatusCode: http.StatusOK,
		},
		{
			desc:               "one item in mesh node list",
			mockFile:           "getmeshnodes_one_mesh_pod.yaml",
			expectedBody:       "[{\"Name\":\"mesh-pod-1\",\"IP\":\"10.4.3.2\",\"Ready\":true}]\n",
			expectedStatusCode: http.StatusOK,
		},
		{
			desc:               "one item in mesh node list with non ready pod",
			mockFile:           "getmeshnodes_one_nonready_mesh_pod.yaml",
			expectedBody:       "[{\"Name\":\"mesh-pod-1\",\"IP\":\"10.4.19.1\",\"Ready\":false}]\n",
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			config := safe.Safe{}
			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			deploylog := deploylog.NewDeployLog(log, 1000)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			clientMock := k8s.NewClientMock(ctx.Done(), test.mockFile, false)
			api := NewAPI(log, 9000, localhost, &config, deploylog, clientMock.PodLister, "foo")
			res := httptest.NewRecorder()
			req := testhelpers.MustNewRequest(http.MethodGet, "/api/status/nodes", nil)

			api.getMeshNodes(res, req)

			assert.Equal(t, test.expectedBody, res.Body.String())
			assert.Equal(t, test.expectedStatusCode, res.Code)
		})
	}
}

func TestGetMeshNodeConfiguration(t *testing.T) {
	testCases := []struct {
		desc               string
		mockFile           string
		expectedBody       string
		expectedStatusCode int
		podError           bool
	}{
		{
			desc:               "simple mesh node configuration",
			mockFile:           "getmeshnodeconfiguration_simple.yaml",
			expectedBody:       "{test_configuration_json}",
			expectedStatusCode: http.StatusOK,
		},
		{
			desc:               "pod not found",
			mockFile:           "getmeshnodeconfiguration_empty.yaml",
			expectedBody:       "unable to find pod: mesh-pod-1",
			expectedStatusCode: http.StatusNotFound,
		},
	}

	apiServer := startTestAPIServer("8080", http.StatusOK, []byte("{test_configuration_json}"))
	defer apiServer.Close()

	for _, test := range testCases {
		config := safe.Safe{}
		log := logrus.New()

		log.SetOutput(os.Stdout)
		log.SetLevel(logrus.DebugLevel)

		deploylog := deploylog.NewDeployLog(log, 1000)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		clientMock := k8s.NewClientMock(ctx.Done(), test.mockFile, false)
		api := NewAPI(log, 9000, localhost, &config, deploylog, clientMock.PodLister, "foo")
		res := httptest.NewRecorder()
		req := testhelpers.MustNewRequest(http.MethodGet, "/api/status/node/mesh-pod-1/configuration", nil)

		//fake gorilla/mux vars
		vars := map[string]string{
			"node": "mesh-pod-1",
		}

		req = mux.SetURLVars(req, vars)

		api.getMeshNodeConfiguration(res, req)

		assert.Equal(t, test.expectedBody, res.Body.String())
		assert.Equal(t, test.expectedStatusCode, res.Code)
	}
}

func startTestAPIServer(port string, statusCode int, bodyData []byte) (ts *httptest.Server) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write(bodyData)
	})
	listener, err := net.Listen("tcp", "127.0.0.1:"+port)

	if err != nil {
		panic(err)
	}

	ts = &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	ts.Start()

	return ts
}
