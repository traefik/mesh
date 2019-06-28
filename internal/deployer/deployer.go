package deployer

import (
	"encoding/json"
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
}

// Init the deployer.
func (d *Deployer) Init() error {
	log.Info("Initializing Deployer")
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
		// Only remove item from queue on successful deploy
		d.configQueue.Forget(item)
	}

	// keep the worker loop running by returning true if there are queue objects remaining
	return d.configQueue.Len() > 0
}

func (d *Deployer) deployConfiguration(c *config.Configuration) bool {

	// Only return true on successful deployment,
	// or else the configuration will be removed from the queue
	return d.deployConfigmap(c) && d.deployAPI(c)
}

func (d *Deployer) deployConfigmap(c *config.Configuration) bool {

	var jsonDataRaw []byte
	jsonDataRaw, err := json.Marshal(c)
	if err != nil {
		log.Errorf("Could not marshal configuration: %s", err)
		return false
	}

	jsonData := string(jsonDataRaw)

	configmap, exists, err := d.client.GetConfigmap(k8s.MeshNamespace, "i3o-config")
	if err != nil {
		log.Errorf("Could not get configmap: %s", err)
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
			log.Errorf("Could not create configmap: %s", err)
			return false
		}
		// Only return true on successful deployment,
		// or else the configuration will be removed from the queue
		return true
	}

	// Configmap exists, deep copy then update
	newConfigmap := configmap.DeepCopy()
	newConfigmap.Data["config.yml"] = jsonData

	_, err = d.client.UpdateConfigmap(newConfigmap)
	if err != nil {
		log.Errorf("Could not update configmap: %s", err)
		return false
	}
	// Only return true on successful deployment,
	// or else the configuration will be removed from the queue
	return true
}

func (d *Deployer) deployAPI(c *config.Configuration) bool {

	// Only return true on successful deployment,
	// FIXME: This will need to be implemented when
	// Traefik v2 has an api provider
	return true
}
