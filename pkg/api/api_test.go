package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	localhost = "127.0.0.1"
)

func TestEnableReadiness(t *testing.T) {
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	client := fake.NewSimpleClientset()
	api, err := NewAPI(log, 9000, localhost, client, "foo")

	require.NoError(t, err)
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
			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			client := fake.NewSimpleClientset()
			api, err := NewAPI(log, 9000, localhost, client, "foo")

			require.NoError(t, err)
			api.readiness.Set(test.readiness)

			res := httptest.NewRecorder()

			req, err := http.NewRequest(http.MethodGet, "/api/status/readiness", nil)
			if err != nil {
				require.NoError(t, err)
				return
			}

			api.getReadiness(res, req)

			assert.Equal(t, test.expectedStatusCode, res.Code)
		})
	}
}

func TestGetCurrentConfiguration(t *testing.T) {
	log := logrus.New()

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.DebugLevel)

	client := fake.NewSimpleClientset()
	api, err := NewAPI(log, 9000, localhost, client, "foo")

	require.NoError(t, err)
	api.configuration.Set("foo")

	res := httptest.NewRecorder()

	req, err := http.NewRequest(http.MethodGet, "/api/configuration/current", nil)
	if err != nil {
		require.NoError(t, err)
		return
	}

	api.getCurrentConfiguration(res, req)

	assert.Equal(t, "\"foo\"\n", res.Body.String())
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
			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			clientMock := k8s.NewClientMock(t, test.mockFile)
			api, err := NewAPI(log, 9000, localhost, clientMock.KubernetesClient(), "foo")

			require.NoError(t, err)

			res := httptest.NewRecorder()

			req, err := http.NewRequest(http.MethodGet, "/api/status/nodes", nil)
			if err != nil {
				require.NoError(t, err)
				return
			}

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
			expectedBody:       "\n",
			expectedStatusCode: http.StatusNotFound,
		},
	}

	apiServer := startTestAPIServer("8080", http.StatusOK, []byte("{test_configuration_json}"))
	defer apiServer.Close()

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			log := logrus.New()

			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)

			clientMock := k8s.NewClientMock(t, test.mockFile)
			api, err := NewAPI(log, 9000, localhost, clientMock.KubernetesClient(), "foo")

			require.NoError(t, err)

			res := httptest.NewRecorder()

			req, err := http.NewRequest(http.MethodGet, "/api/status/node/mesh-pod-1/configuration", nil)
			if err != nil {
				require.NoError(t, err)
				return
			}

			// fake gorilla/mux vars
			vars := map[string]string{
				"node": "mesh-pod-1",
			}

			req = mux.SetURLVars(req, vars)

			api.getMeshNodeConfiguration(res, req)

			assert.Equal(t, test.expectedBody, res.Body.String())
			assert.Equal(t, test.expectedStatusCode, res.Code)
		})
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
