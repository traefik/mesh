package controller

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	kubeerror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	listers "k8s.io/client-go/listers/core/v1"
)

// API is an implementation of an api.
type API struct {
	router            *mux.Router
	readiness         bool
	lastConfiguration *safe.Safe
	apiPort           int
	apiIP             string
	deployLog         *DeployLog
	meshNamespace     string
	podLister         listers.PodLister
}

type podInfo struct {
	Name  string
	IP    string
	Ready bool
}

// NewAPI creates a new api.
func NewAPI(apiPort int, apiIP string, lastConfiguration *safe.Safe, deployLog *DeployLog, podLister listers.PodLister, meshNamespace string) *API {
	a := &API{
		readiness:         false,
		lastConfiguration: lastConfiguration,
		apiPort:           apiPort,
		apiIP:             apiIP,
		deployLog:         deployLog,
		podLister:         podLister,
		meshNamespace:     meshNamespace,
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
	a.router.HandleFunc("/api/status/nodes", a.getMeshNodes)
	a.router.HandleFunc("/api/status/node/{node}/configuration", a.getMeshNodeConfiguration)
	a.router.HandleFunc("/api/status/readiness", a.getReadiness)
	a.router.HandleFunc("/api/log/deployment", a.getDeployLog)

	return nil
}

// Start runs the API.
func (a *API) Start() {
	log.Debugln("API.Start")

	go a.Run()
}

// Run wraps the listenAndServe method.
func (a *API) Run() {
	log.Error(http.ListenAndServe(fmt.Sprintf("%s:%d", a.apiIP, a.apiPort), a.router))
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
	entries := a.deployLog.GetLog()

	data, err := json.Marshal(entries)
	if err != nil {
		writeErrorResponse(w, fmt.Sprintf("unable to marshal deploy log entries: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if _, err := w.Write(data); err != nil {
		log.Error(err)
	}
}

// getMeshNodes returns a list of mesh nodes visible from the controller, and some basic readiness info.
func (a *API) getMeshNodes(w http.ResponseWriter, r *http.Request) {
	podInfoList := []podInfo{}

	sel := labels.Everything()

	requirement, err := labels.NewRequirement("component", selection.Equals, []string{"maesh-mesh"})
	if err != nil {
		log.Error(err)
	}

	sel = sel.Add(*requirement)

	podList, err := a.podLister.Pods(a.meshNamespace).List(sel)
	if err != nil {
		writeErrorResponse(w, fmt.Sprintf("unable to retrieve pod list: %v", err), http.StatusInternalServerError)
		return
	}

	for _, pod := range podList {
		readiness := true

		for _, status := range pod.Status.ContainerStatuses {
			if !status.Ready {
				// If there is a non-ready container, pod is not ready.
				readiness = false
				break
			}
		}

		p := podInfo{
			Name:  pod.Name,
			IP:    pod.Status.PodIP,
			Ready: readiness,
		}
		podInfoList = append(podInfoList, p)
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(podInfoList); err != nil {
		log.Error(err)
	}
}

// getMeshNodeConfiguration returns the configuration for a named pod.
func (a *API) getMeshNodeConfiguration(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	pod, err := a.podLister.Pods(a.meshNamespace).Get(vars["node"])
	if err != nil {
		if kubeerror.IsNotFound(err) {
			writeErrorResponse(w, fmt.Sprintf("unable to find pod: %s", vars["node"]), http.StatusNotFound)
			return
		}

		writeErrorResponse(w, fmt.Sprintf("unable to retrieve pod: %v", err), http.StatusInternalServerError)

		return
	}

	resp, err := http.Get(fmt.Sprintf("http://%s:8080/api/rawdata", pod.Status.PodIP))
	if err != nil {
		writeErrorResponse(w, fmt.Sprintf("unable to get configuration from pod: %v", err), http.StatusBadGateway)
		return
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		writeErrorResponse(w, fmt.Sprintf("unable to get configuration response body from pod: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if _, err := w.Write(body); err != nil {
		log.Error(err)
	}
}

func writeErrorResponse(w http.ResponseWriter, errorMessage string, status int) {
	w.WriteHeader(status)
	log.Error(errorMessage)

	w.Header().Set("Content-Type", "text/plain; charset=us-ascii")

	if _, err := w.Write([]byte(errorMessage)); err != nil {
		log.Error(err)
	}
}
