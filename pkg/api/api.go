package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/traefik/mesh/v2/pkg/provider"
	"github.com/traefik/mesh/v2/pkg/safe"
	"github.com/traefik/mesh/v2/pkg/topology"
	"github.com/traefik/traefik/v2/pkg/config/dynamic"
)

// API is an implementation of an api.
type API struct {
	http.Server

	readiness     *safe.Safe
	configuration *safe.Safe
	topology      *safe.Safe

	namespace string
	logger    logrus.FieldLogger
}

// NewAPI creates a new api.
func NewAPI(logger logrus.FieldLogger, port int32, host, namespace string) *API {
	router := mux.NewRouter()

	api := &API{
		Server: http.Server{
			Addr:         fmt.Sprintf("%s:%d", host, port),
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
			Handler:      router,
		},
		configuration: safe.New(provider.NewDefaultDynamicConfig()),
		topology:      safe.New(topology.NewTopology()),
		readiness:     safe.New(false),
		namespace:     namespace,
		logger:        logger,
	}

	router.HandleFunc("/api/configuration", api.getConfiguration)
	router.HandleFunc("/api/topology", api.getTopology)
	router.HandleFunc("/api/ready", api.getReadiness)

	return api
}

// SetReadiness sets the readiness flag in the API.
func (a *API) SetReadiness(isReady bool) {
	a.readiness.Set(isReady)
	a.logger.Debugf("API readiness: %t", isReady)
}

// SetConfiguration sets the current dynamic configuration.
func (a *API) SetConfiguration(cfg *dynamic.Configuration) {
	a.configuration.Set(cfg)
}

// SetTopology sets the current topology.
func (a *API) SetTopology(topo *topology.Topology) {
	a.topology.Set(topo)
}

// getConfiguration returns the current configuration.
func (a *API) getConfiguration(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(a.configuration.Get()); err != nil {
		a.logger.Errorf("Unable to serialize configuration: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
	}
}

// getTopology returns the current topology.
func (a *API) getTopology(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(a.topology.Get()); err != nil {
		a.logger.Errorf("Unable to serialize topology: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
	}
}

// getReadiness returns the current readiness value, and sets the status code to 500 if not ready.
func (a *API) getReadiness(w http.ResponseWriter, _ *http.Request) {
	isReady, _ := a.readiness.Get().(bool)
	if !isReady {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(isReady); err != nil {
		a.logger.Errorf("Unable to serialize readiness: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
	}
}
