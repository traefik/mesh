package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/containous/maesh/pkg/k8s"
	"github.com/containous/maesh/pkg/provider"
	"github.com/containous/maesh/pkg/topology"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/containous/traefik/v2/pkg/safe"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	kubeerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listers "k8s.io/client-go/listers/core/v1"
)

// API is an implementation of an api.
type API struct {
	http.Server

	readiness     *safe.Safe
	configuration *safe.Safe
	topology      *safe.Safe

	meshNamespace string
	podLister     listers.PodLister
	log           logrus.FieldLogger
}

type podInfo struct {
	Name  string
	IP    string
	Ready bool
}

// NewAPI creates a new api.
func NewAPI(log logrus.FieldLogger, apiPort int32, apiHost string, client kubernetes.Interface, meshNamespace string) (*API, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{"component": "maesh-mesh"},
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create label selector: %w", err)
	}

	informerFactory := informers.NewSharedInformerFactoryWithOptions(client, k8s.ResyncPeriod,
		informers.WithNamespace(meshNamespace),
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = selector.String()
		}))

	podLister := informerFactory.Core().V1().Pods().Lister()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	informerFactory.Start(ctx.Done())

	for t, ok := range informerFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return nil, fmt.Errorf("timed out while waiting for informer cache to sync: %s", t)
		}
	}

	router := mux.NewRouter()

	a := &API{
		Server: http.Server{
			Addr:         fmt.Sprintf("%s:%d", apiHost, apiPort),
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
			Handler:      router,
		},
		configuration: safe.New(provider.NewDefaultDynamicConfig()),
		topology:      safe.New(topology.NewTopology()),
		readiness:     safe.New(false),
		podLister:     podLister,
		meshNamespace: meshNamespace,
		log:           log,
	}

	router.HandleFunc("/api/configuration/current", a.getCurrentConfiguration)
	router.HandleFunc("/api/topology/current", a.getCurrentTopology)
	router.HandleFunc("/api/status/nodes", a.getMeshNodes)
	router.HandleFunc("/api/status/node/{node}/configuration", a.getMeshNodeConfiguration)
	router.HandleFunc("/api/status/readiness", a.getReadiness)

	return a, nil
}

// SetReadiness sets the readiness flag in the API.
func (a *API) SetReadiness(isReady bool) {
	readiness, ok := a.readiness.Get().(bool)

	if ok && readiness == isReady {
		return
	}

	a.readiness.Set(isReady)

	if isReady {
		a.log.Debug("API readiness enabled")
	} else {
		a.log.Debug("API readiness disabled")
	}
}

// SetConfig sets the current dynamic configuration.
func (a *API) SetConfig(cfg *dynamic.Configuration) {
	a.configuration.Set(cfg)
}

// SetTopology sets the current topology.
func (a *API) SetTopology(topo *topology.Topology) {
	a.topology.Set(topo)
}

// getCurrentConfiguration returns the current configuration.
func (a *API) getCurrentConfiguration(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(a.configuration.Get()); err != nil {
		a.log.Errorf("Unable to serialize dynamic configuration: %v", err)
		a.writeErrorResponse(w, nil, http.StatusInternalServerError)

		return
	}
}

// getCurrentTopology returns the current topology.
func (a *API) getCurrentTopology(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(a.topology.Get()); err != nil {
		a.log.Errorf("Unable to serialize topology: %v", err)
		a.writeErrorResponse(w, nil, http.StatusInternalServerError)

		return
	}
}

// getReadiness returns the current readiness value, and sets the status code to 500 if not ready.
func (a *API) getReadiness(w http.ResponseWriter, _ *http.Request) {
	isReady, _ := a.readiness.Get().(bool)
	if !isReady {
		a.writeErrorResponse(w, nil, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(isReady); err != nil {
		a.log.Errorf("Unable to serialize readiness: %v", err)
		a.writeErrorResponse(w, nil, http.StatusInternalServerError)

		return
	}
}

// getMeshNodes returns a list of mesh nodes visible from the controller, and some basic readiness info.
func (a *API) getMeshNodes(w http.ResponseWriter, _ *http.Request) {
	// Make sure it returns an empty array and not "null".
	podInfoList := make([]podInfo, 0)

	podList, err := a.podLister.List(labels.Everything())
	if err != nil {
		a.log.Errorf("Unable to retrieve pod list: %v", err)
		a.writeErrorResponse(w, nil, http.StatusInternalServerError)

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
		a.log.Errorf("Unable to serialize mesh nodes: %v", err)
		a.writeErrorResponse(w, nil, http.StatusInternalServerError)

		return
	}
}

// getMeshNodeConfiguration returns the configuration for a named pod.
func (a *API) getMeshNodeConfiguration(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	pod, err := a.podLister.Pods(a.meshNamespace).Get(vars["node"])
	if err != nil {
		if kubeerror.IsNotFound(err) {
			a.writeErrorResponse(w, fmt.Errorf("unable to find pod: %s", vars["node"]), http.StatusNotFound)
			return
		}

		a.writeErrorResponse(w, nil, http.StatusInternalServerError)

		return
	}

	resp, err := http.Get(fmt.Sprintf("http://%s:8080/api/rawdata", pod.Status.PodIP))
	if err != nil {
		a.log.Errorf("Unable to get configuration from pod: %v", err)
		a.writeErrorResponse(w, nil, http.StatusBadGateway)

		return
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			a.log.Errorf("Unable to close response body: %w", closeErr)
		}
	}()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		a.log.Errorf("Unable to get configuration response body from pod: %v", err)
		a.writeErrorResponse(w, nil, http.StatusBadGateway)

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if _, err := w.Write(body); err != nil {
		a.log.Errorf("Unable to write mesh nodes: %v", err)
		a.writeErrorResponse(w, nil, http.StatusInternalServerError)

		return
	}
}

func (a *API) writeErrorResponse(w http.ResponseWriter, err error, status int) {
	w.WriteHeader(status)

	if err == nil {
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=us-ascii")

	if _, err = w.Write([]byte(err.Error())); err != nil {
		a.log.Error(err)
	}
}
