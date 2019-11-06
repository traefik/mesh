package controller

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

// API is an implementation of an api.
type API struct {
	router            *mux.Router
	readiness         bool
	lastConfiguration *safe.Safe
	apiPort           int
	deployLog         *DeployLog
}

// NewAPI creates a new api.
func NewAPI(apiPort int, lastConfiguration *safe.Safe, deployLog *DeployLog) *API {
	a := &API{
		readiness:         false,
		lastConfiguration: lastConfiguration,
		apiPort:           apiPort,
		deployLog:         deployLog,
	}

	if err := a.Init(); err != nil {
		log.Errorln("Could not initialize API")
	}

	return a
}

// Init handles any api initialization.
func (a *API) Init() error {
	log.Debugln("API.Init")

	a.router = mux.NewRouter()

	a.router.HandleFunc("/api/configuration/current", a.getCurrentConfiguration)
	a.router.HandleFunc("/api/status/readiness", a.getReadiness)
	a.router.HandleFunc("/api/log/deploylog", a.getDeployLog)

	return nil
}

// Start runs the API.
func (a *API) Start() {
	log.Debugln("API.Start")

	go a.Run()
}

// Run wraps the listenAndServe method.
func (a *API) Run() {
	log.Error(http.ListenAndServe(fmt.Sprintf(":%d", a.apiPort), a.router))
}

// EnableReadiness enables the readiness flag in the API.
func (a *API) EnableReadiness() {
	if !a.readiness {
		log.Debug("Controller Readiness enabled")

		a.readiness = true
	}
}

// getCurrentConfiguration returns the current configuration.
func (a *API) getCurrentConfiguration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(a.lastConfiguration.Get()); err != nil {
		log.Error(err)
	}
}

// getReadiness returns the current readiness value, and sets the status code to 500 if not ready.
func (a *API) getReadiness(w http.ResponseWriter, r *http.Request) {
	if !a.readiness {
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(a.readiness); err != nil {
		log.Error(err)
	}
}

// getDeployLog returns the current deploylog.
func (a *API) getDeployLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if _, err := w.Write(a.deployLog.GetLog()); err != nil {
		log.Error(err)
	}
}
