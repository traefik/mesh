package deployer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v3"
	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/maesh/internal/message"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/containous/traefik/v2/pkg/safe"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
)

const maxRetry = 3

// Deployer holds a client to access the provider.
type Deployer struct {
	client        k8s.CoreV1Client
	configQueue   workqueue.RateLimitingInterface
	deployQueue   workqueue.RateLimitingInterface
	meshNamespace string
}

// Init the deployer.
func (d *Deployer) Init() error {
	log.Info("Initializing Deployer")
	d.deployQueue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	return nil
}

// New creates a new deployer.
func New(client k8s.CoreV1Client, configQueue workqueue.RateLimitingInterface, meshNamespace string) *Deployer {
	d := &Deployer{
		client:        client,
		configQueue:   configQueue,
		meshNamespace: meshNamespace,
	}

	if err := d.Init(); err != nil {
		log.Errorln("Could not initialize Deployer")
	}

	return d
}

// Run is the main entrypoint for the deployer.
func (d *Deployer) Run(stopCh <-chan struct{}) {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()

	// Start the deployQueue processing
	go d.processDeployQueue(stopCh)

	// run the runWorker method every second with a stop channel
	wait.Until(d.runWorker, time.Second, stopCh)
}

// runWorker executes the loop to process new items added to the queue
func (d *Deployer) runWorker() {
	log.Debug("Deployer: starting")

	// invoke processNextItem to fetch and consume the next change
	// to a watched or listed resource
	for d.processNextItem() {
		log.Debug("Deployer.runWorker: processing next item")
	}

	log.Debug("Deployer.runWorker: completed")
}

// processNextItem retrieves each queued item and takes the
// necessary handler action based off of the event type.
func (d *Deployer) processNextItem() bool {
	log.Debug("Deployer - Config Processing Waiting for next item to process...")
	if d.configQueue.Len() > 0 {
		log.Debugf("Config queue length: %d", d.configQueue.Len())
	}
	// fetch the next item (blocking) from the queue to process or
	// if a shutdown is requested then return out of this to stop
	// processing
	item, quit := d.configQueue.Get()

	// stop the worker loop from running as this indicates we
	// have sent a shutdown message that the queue has indicated
	// from the Get method
	if quit {
		return false
	}

	defer d.configQueue.Done(item)

	event := item.(message.Config)

	if d.deployConfiguration(event.Config) {
		// Only remove the configuration if the config was successfully added to the deploy queue
		d.configQueue.Forget(item)
	}

	// keep the worker loop running by returning true if there are queue objects remaining
	return d.configQueue.Len() > 0
}

// deployConfiguration takes the configuration, and adds it into the deploy queue for each affected
// mesh node. This allows nodes to retry individually.
func (d *Deployer) deployConfiguration(c *dynamic.Configuration) bool {
	// Make a copy to deploy, so changes to the main configuration don't propagate
	deployConfig := c.DeepCopy()

	podList, err := d.client.ListPodWithOptions(d.meshNamespace, metav1.ListOptions{
		LabelSelector: "component==maesh-mesh",
	})
	if err != nil {
		log.Errorf("Could not retrieve pod list: %v", err)
		return false
	}

	if len(podList.Items) == 0 {
		log.Errorf("Could not find any active mesh pods to deploy config : %+v", c.HTTP)
		return false
	}

	for _, pod := range podList.Items {
		log.Debugf("Add configuration to deploy queue for pod %s with IP %s", pod.Name, pod.Status.PodIP)

		d.DeployToPod(pod.Name, pod.Status.PodIP, deployConfig)
	}

	return true
}

// DeployToPod takes the configuration, and adds it into the deploy queue for a pod.
func (d *Deployer) DeployToPod(name, ip string, c *dynamic.Configuration) {
	if name == "" && ip == "" {
		// If there is no name and ip, then just return.
		return
	}

	// Make a copy to deploy, so changes to the main configuration don't propagate
	deployConfig := c.DeepCopy()

	log.Infof("Adding configuration to deploy queue for pod %s, with IP: %s", name, ip)
	d.deployQueue.Add(message.Deploy{
		PodName: name,
		PodIP:   ip,
		Config:  deployConfig,
	})
}

func (d *Deployer) deployAPI(m message.Deploy) bool {
	if m.PodIP == "" {
		// Invalid deployment message, return true so that the deploy message doesn't get retried.
		return true
	}

	log.Debugf("Deploying configuration to pod %s with IP %s", m.PodName, m.PodIP)
	b, err := json.Marshal(m.Config)
	if err != nil {
		log.Errorf("Unable to marshal configuration: %v", err)
		return false
	}

	currentVersion, err := m.GetVersion()
	if err != nil {
		log.Errorf("Could not get current configuration version: %v", err)
		return false
	}

	activeVersion, exists, err := getDeployedVersion(m.PodIP)
	if err != nil {
		log.Errorf("Could not get deployed configuration version: %v", err)
		return false
	}
	if exists {
		log.Debugf("Currently deployed version for pod %s: %s", m.PodName, activeVersion)

		if currentVersion.Before(activeVersion) {
			// The version we are trying to deploy is outdated.
			// Return true, so that it will be removed from the deploy queue.
			log.Debugf("Skipping outdated configuration: %v", currentVersion)
			return true
		}
		if currentVersion.Equal(activeVersion) {
			// The version we are trying to deploy is already deployed.
			// Return true, so that it will be removed from the deploy queue.
			log.Debugf("Skipping already deployed configuration: %v", currentVersion)
			return true
		}
		log.Debugf("Deploying configuration version for pod %s: %s", m.PodName, currentVersion)
	}

	url := fmt.Sprintf("http://%s:8080/api/providers/rest", m.PodIP)

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(b))
	if err != nil {
		log.Errorf("Could not create request: %v", err)
		return false
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
		if _, bodyErr := ioutil.ReadAll(resp.Body); bodyErr != nil {
			log.Errorf("Unable to read response body: %v", bodyErr)
			return false
		}
		return waitForDeployToProcess(currentVersion, m.PodName, m.PodIP)
	}
	if err != nil {
		log.Errorf("Unable to deploy configuration: %v", err)
	}

	return false
}

// waitForDeployToProcess loops until the deployed version is reported
func waitForDeployToProcess(currentVersion time.Time, name, ip string) bool {
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = 10 * time.Second
	deployError := backoff.Retry(safe.OperationWithRecover(func() error {
		// Configuration should have deployed successfully, confirm version match.
		newVersion, exists, newErr := getDeployedVersion(ip)
		if newErr != nil {
			return fmt.Errorf("could not get newly deployed configuration version: %v", newErr)
		}
		if exists {
			if currentVersion.Equal(newVersion) {
				// The version we are trying to deploy is confirmed.
				// Return nil, to break out of the ebo.
				return nil
			}
		}
		return fmt.Errorf("deployment was not successful")
	}), ebo)

	if deployError == nil {
		// The version we are trying to deploy is confirmed.
		// Return true, so that it will be removed from the deploy queue.
		log.Debugf("Successfully deployed version for pod %s: %s", name, currentVersion)
		return true
	}
	return false
}

// processDeployQueue is the main entrypoint for the deployer to deploy configurations.
func (d *Deployer) processDeployQueue(stopCh <-chan struct{}) {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()

	// run the runWorker method every second with a stop channel
	wait.Until(d.processDeployQueueWorker, time.Second, stopCh)
}

// processDeployQueueWorker executes the loop to process new items added to the queue
func (d *Deployer) processDeployQueueWorker() {
	log.Debug("Deployer process deploy queue: starting")

	// invoke pprocessDeployQueueNextItem to fetch and consume the next change
	// to a watched or listed resource
	for d.processDeployQueueNextItem() {
		log.Debug("Deployer.runWorker: processing next item")
	}

	log.Debug("Deployer process deploy queue: completed")
}

// processDeployQueueNextItem retrieves each queued item and takes the
// necessary handler action based off of the event type.
func (d *Deployer) processDeployQueueNextItem() bool {
	log.Debug("Deployer - Deploy Processing Waiting for next item to process...")
	if d.deployQueue.Len() > 0 {
		log.Debugf("Deploy queue length: %d", d.deployQueue.Len())
	}

	// fetch the next item (blocking) from the queue to process or
	// if a shutdown is requested then return out of this to stop
	// processing
	item, quit := d.deployQueue.Get()

	// stop the worker loop from running as this indicates we
	// have sent a shutdown message that the queue has indicated
	// from the Get method
	if quit {
		return false
	}

	defer d.deployQueue.Done(item)

	deployConfig := item.(message.Deploy)
	log.Debug("Deploying configuration to pod...")
	if d.deployAPI(deployConfig) {
		// Only remove item from queue on successful deploy.
		d.deployQueue.Forget(item)
		return d.deployQueue.Len() > 0
	}

	if d.deployQueue.NumRequeues(item) < maxRetry {
		// Deploy to API failed, re-add to the queue.
		d.deployQueue.AddRateLimited(item)
	}

	// Keep the worker loop running by returning true if there are queue objects remaining.
	return d.deployQueue.Len() > 0
}

func getDeployedVersion(ip string) (time.Time, bool, error) {
	url := fmt.Sprintf("http://%s:8080/api/rawdata", ip)
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Errorf("Could not create request: %v", err)
		return time.Now(), false, err
	}
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
		body, bodyErr := ioutil.ReadAll(resp.Body)
		if bodyErr != nil {
			log.Errorf("Unable to read response body: %v", bodyErr)
			return time.Now(), false, bodyErr
		}

		trimmedBody := strings.TrimRight(string(body), "\n")
		data := new(dynamic.HTTPConfiguration)
		if unmarshalErr := json.Unmarshal([]byte(trimmedBody), data); err != nil {
			log.Errorf("Unable to parse response body: %v", unmarshalErr)
			return time.Now(), false, unmarshalErr
		}

		var version int64
		var timeError error

		if len(data.Services) == 0 {
			return time.Now(), false, nil
		}

		versionKey := message.ConfigServiceVersionKey + "@rest"
		if value, exists := data.Services[versionKey]; exists {
			version, timeError = strconv.ParseInt(value.LoadBalancer.Servers[0].URL, 10, 64)
			if timeError != nil {
				return time.Now(), false, timeError
			}
			return time.Unix(0, version), true, nil
		}
		return time.Now(), false, nil
	}
	log.Errorf("Got no response: %v", err)
	return time.Now(), false, err
}
