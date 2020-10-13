package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/traefik/mesh/v2/pkg/k8s"
	"github.com/traefik/mesh/v2/pkg/provider"
	"github.com/traefik/mesh/v2/pkg/safe"
	"github.com/traefik/mesh/v2/pkg/topology"
	"github.com/traefik/traefik/v2/pkg/config/dynamic"
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

	namespace string
	podLister listers.PodLister
	logger    logrus.FieldLogger
}

type podInfo struct {
	Name  string
	IP    string
	Ready bool
}

// NewAPI creates a new api.
func NewAPI(logger logrus.FieldLogger, port int32, host string, client kubernetes.Interface, namespace string) (*API, error) {
	informerFactory := informers.NewSharedInformerFactoryWithOptions(client, k8s.ResyncPeriod,
		informers.WithNamespace(namespace),
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = k8s.ProxySelector().String()
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
		podLister:     podLister,
		namespace:     namespace,
		logger:        logger,
	}

	router.HandleFunc("/api/configuration/current", api.getCurrentConfiguration)
	router.HandleFunc("/api/topology/current", api.getCurrentTopology)
	router.HandleFunc("/api/status/nodes", api.getMeshNodes)
	router.HandleFunc("/api/status/node/{node}/configuration", api.getMeshNodeConfiguration)
	router.HandleFunc("/api/status/readiness", api.getReadiness)

	return api, nil
}

// SetReadiness sets the readiness flag in the API.
func (a *API) SetReadiness(isReady bool) {
	a.readiness.Set(isReady)
	a.logger.Debugf("API readiness: %t", isReady)
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
		a.logger.Errorf("Unable to serialize dynamic configuration: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
	}
}

// getCurrentTopology returns the current topology.
func (a *API) getCurrentTopology(w http.ResponseWriter, _ *http.Request) {
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

// getMeshNodes returns a list of mesh nodes visible from the controller, and some basic readiness info.
func (a *API) getMeshNodes(w http.ResponseWriter, _ *http.Request) {
	podList, err := a.podLister.List(labels.Everything())
	if err != nil {
		a.logger.Errorf("Unable to retrieve pod list: %v", err)
		http.Error(w, "", http.StatusInternalServerError)

		return
	}

	podInfoList := make([]podInfo, len(podList))

	for i, pod := range podList {
		readiness := true

		for _, status := range pod.Status.ContainerStatuses {
			if !status.Ready {
				// If there is a non-ready container, pod is not ready.
				readiness = false
				break
			}
		}

		podInfoList[i] = podInfo{
			Name:  pod.Name,
			IP:    pod.Status.PodIP,
			Ready: readiness,
		}
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(podInfoList); err != nil {
		a.logger.Errorf("Unable to serialize mesh nodes: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
	}
}

// getMeshNodeConfiguration returns the configuration for a named pod.
func (a *API) getMeshNodeConfiguration(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	pod, err := a.podLister.Pods(a.namespace).Get(vars["node"])
	if err != nil {
		if kubeerror.IsNotFound(err) {
			http.Error(w, "", http.StatusNotFound)

			return
		}

		http.Error(w, "", http.StatusInternalServerError)

		return
	}

	resp, err := http.Get(fmt.Sprintf("http://%s:8080/api/rawdata", pod.Status.PodIP))
	if err != nil {
		a.logger.Errorf("Unable to get configuration from pod %q: %v", pod.Name, err)
		http.Error(w, "", http.StatusBadGateway)

		return
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			a.logger.Errorf("Unable to close response body: %v", closeErr)
		}
	}()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		a.logger.Errorf("Unable to get configuration response body from pod %q: %v", pod.Name, err)
		http.Error(w, "", http.StatusBadGateway)

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if _, err := w.Write(body); err != nil {
		a.logger.Errorf("Unable to write mesh nodes: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
	}
}
