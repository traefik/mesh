package deployer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/containous/i3o/internal/k8s"
	"github.com/containous/traefik/pkg/config"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
)

// Deployer holds a client to access the provider.
type Deployer struct {
	client      k8s.CoreV1Client
	configQueue workqueue.RateLimitingInterface
	deployQueue workqueue.RateLimitingInterface
}

// Init the deployer.
func (d *Deployer) Init() error {
	log.Info("Initializing Deployer")
	d.deployQueue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	return nil
}

// New creates a new deployer.
func New(client k8s.CoreV1Client, configQueue workqueue.RateLimitingInterface) *Deployer {
	d := &Deployer{
		client:      client,
		configQueue: configQueue,
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
	log.Debug("Deployer Waiting for next item to process...")

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

	event := item.(*config.Configuration)

	if d.deployConfiguration(event) {
		// Only remove the configuration if the config was successfully added to the deploy queue
		d.configQueue.Forget(item)
	}

	// keep the worker loop running by returning true if there are queue objects remaining
	return d.configQueue.Len() > 0
}

// deployConfiguration takes the configuration, and adds it into the deploy queue for each affected
// mesh node. This allows nodes to retry individually.
func (d *Deployer) deployConfiguration(c *config.Configuration) bool {

	podList, err := d.client.ListPodWithOptions(k8s.MeshNamespace, metav1.ListOptions{
		LabelSelector: "component==i3o-mesh",
	})
	if err != nil {
		log.Errorf("Could not retrieve pod list: %v", err)
		return false
	}

	for _, pod := range podList.Items {
		log.Debugf("Add configuration to deploy queue for pod %q with IP %s \n", pod.Name, pod.Status.PodIP)

		messge := Message{
			PodName: pod.Name,
			PodIP:   pod.Status.PodIP,
			Config:  c,
		}

		d.deployQueue.Add(messge)
	}

	// Add the configmap update to the deploy queue
	message := Message{
		ConfigmapDeploy: true,
		Config:          c,
	}
	d.deployQueue.Add(message)

	return true
}

func (d *Deployer) deployConfigmap(m Message) bool {

	var jsonDataRaw []byte
	jsonDataRaw, err := json.Marshal(m.Config)
	if err != nil {
		log.Errorf("Could not marshal configuration: %s", err)
		return false
	}

	jsonData := string(jsonDataRaw)

	configmap, exists, err := d.client.GetConfigmap(k8s.MeshNamespace, "i3o-config")
	if err != nil {
		log.Errorf("Could not get configmap: %v", err)
		return false
	}
	if !exists {
		// Does not exist, create
		newConfigmap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "i3o-config",
				Namespace: k8s.MeshNamespace,
			},
			Data: map[string]string{
				"config.yml": jsonData,
			},
		}

		_, err = d.client.CreateConfigmap(newConfigmap)
		if err != nil {
			log.Errorf("Could not create configmap: %v", err)
			return false
		}
		// Only return true on successful deployment,
		// or else the configuration will be removed from the queue
		return true
	}

	// Configmap exists, deep copy then update
	newConfigmap := configmap.DeepCopy()
	newConfigmap.Data["config.yml"] = jsonData

	if _, err = d.client.UpdateConfigmap(newConfigmap); err != nil {
		log.Errorf("Could not update configmap: %v", err)
		return false
	}
	// Only return true on successful deployment,
	// or else the configuration will be removed from the queue
	return true
}

func (d *Deployer) deployAPI(m Message) bool {

	log.Debugf("Deploying configuration to pod %q with IP %s \n", m.PodName, m.PodIP)
	b, err := json.Marshal(m.Config)
	if err != nil {
		log.Errorf("unable to marshal configuration: %v", err)
	}

	url := fmt.Sprintf("http://%s:8080/api/providers/rest", m.PodIP)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(b))
	if err != nil {
		log.Errorf("unable to deploy configuration: %v", err)
	}
	// FIXME: 404 when posting on the url to deploy configuration
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("unable to read body: %v", err)
	}

	log.Debug(string(body))

	return true
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
	log.Debug("Deployer Waiting for next item to process...")

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

	deployConfig := item.(Message)

	if deployConfig.ConfigmapDeploy {
		log.Debug("Deploying Configmap...")
		if d.deployConfigmap(deployConfig) {
			// Only remove item from queue on successful deploy
			d.deployQueue.Forget(item)
		}
	} else {
		log.Debug("Deploying configuration to pod...")
		if d.deployAPI(deployConfig) {
			// Only remove item from queue on successful deploy
			d.deployQueue.Forget(item)
		}
	}

	// keep the worker loop running by returning true if there are queue objects remaining
	return d.configQueue.Len() > 0
}
