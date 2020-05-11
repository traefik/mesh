package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	kubeerror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	listers "k8s.io/client-go/listers/core/v1"
)

// Ensure the API fits the interface.
var _ Interface = (*API)(nil)

// Interface is an interface to interact with the REST API.
type Interface interface {
	Start()
	EnableReadiness()
}

// API is an implementation of an api.
type API struct {
	log               logrus.FieldLogger
	router            *mux.Router
	readiness         bool
	lastConfiguration *safe.Safe
	apiPort           int32
	apiHost           string
	meshNamespace     string
	podLister         listers.PodLister
}

type podInfo struct {
	Name  string
	IP    string
	Ready bool
}

// NewAPI creates a new api.
func NewAPI(log logrus.FieldLogger, apiPort int32, apiHost string, lastConfiguration *safe.Safe, podLister listers.PodLister, meshNamespace string) *API {
	a := &API{
		log:               log,
		readiness:         false,
		lastConfiguration: lastConfiguration,
		apiPort:           apiPort,
		apiHost:           apiHost,
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
	a.log.Debugln("API.Init")

	a.router = mux.NewRouter()

	a.router.HandleFunc("/api/configuration/current", a.getCurrentConfiguration)
	a.router.HandleFunc("/api/status/nodes", a.getMeshNodes)
	a.router.HandleFunc("/api/status/node/{node}/configuration", a.getMeshNodeConfiguration)
	a.router.HandleFunc("/api/status/readiness", a.getReadiness)

	return nil
}

// Start runs the API.
func (a *API) Start() {
	a.log.Debugln("API.Start")

	go a.Run()
}

// Run wraps the listenAndServe method.
func (a *API) Run() {
	a.log.Error(http.ListenAndServe(fmt.Sprintf("%s:%d", a.apiHost, a.apiPort), a.router))
}

// EnableReadiness enables the readiness flag in the API.
func (a *API) EnableReadiness() {
	if !a.readiness {
		a.log.Debug("Controller Readiness enabled")

		a.readiness = true
	}
}

// getCurrentConfiguration returns the current configuration.
func (a *API) getCurrentConfiguration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(a.lastConfiguration.Get()); err != nil {
		a.log.Error(err)
	}
}

// getReadiness returns the current readiness value, and sets the status code to 500 if not ready.
func (a *API) getReadiness(w http.ResponseWriter, r *http.Request) {
	if !a.readiness {
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(a.readiness); err != nil {
		a.log.Error(err)
	}
}

// getMeshNodes returns a list of mesh nodes visible from the controller, and some basic readiness info.
func (a *API) getMeshNodes(w http.ResponseWriter, r *http.Request) {
	podInfoList := []podInfo{}

	sel := labels.Everything()

	requirement, err := labels.NewRequirement("component", selection.Equals, []string{"maesh-mesh"})
	if err != nil {
		a.log.Error(err)
	}

	sel = sel.Add(*requirement)

	podList, err := a.podLister.Pods(a.meshNamespace).List(sel)
	if err != nil {
		a.writeErrorResponse(w, fmt.Sprintf("unable to retrieve pod list: %v", err), http.StatusInternalServerError)
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
		a.log.Error(err)
	}
}

// getMeshNodeConfiguration returns the configuration for a named pod.
func (a *API) getMeshNodeConfiguration(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	pod, err := a.podLister.Pods(a.meshNamespace).Get(vars["node"])
	if err != nil {
		if kubeerror.IsNotFound(err) {
			a.writeErrorResponse(w, fmt.Sprintf("unable to find pod: %s", vars["node"]), http.StatusNotFound)
			return
		}

		a.writeErrorResponse(w, fmt.Sprintf("unable to retrieve pod: %v", err), http.StatusInternalServerError)

		return
	}

	resp, err := http.Get(fmt.Sprintf("http://%s:8080/api/rawdata", pod.Status.PodIP))
	if err != nil {
		a.writeErrorResponse(w, fmt.Sprintf("unable to get configuration from pod: %v", err), http.StatusBadGateway)
		return
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		a.writeErrorResponse(w, fmt.Sprintf("unable to get configuration response body from pod: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if _, err := w.Write(body); err != nil {
		a.log.Error(err)
	}
}

func (a *API) writeErrorResponse(w http.ResponseWriter, errorMessage string, status int) {
	w.WriteHeader(status)
	a.log.Error(errorMessage)

	w.Header().Set("Content-Type", "text/plain; charset=us-ascii")

	if _, err := w.Write([]byte(errorMessage)); err != nil {
		a.log.Error(err)
	}
}
