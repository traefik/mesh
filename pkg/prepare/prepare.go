package prepare

import (
	"context"
	"fmt"
	"time"

	"github.com/containous/maesh/pkg/dns"
	"github.com/containous/maesh/pkg/k8s"
	accessinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/informers/externalversions"
	specsinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/informers/externalversions"
	splitinformer "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/informers/externalversions"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// Prepare holds the clients for the various resource controllers.
type Prepare struct {
	client    k8s.Client
	dnsClient *dns.Client
	logger    logrus.FieldLogger
}

// NewPrepare returns an initialized prepare object.
func NewPrepare(logger logrus.FieldLogger, client k8s.Client) *Prepare {
	dnsClient := dns.NewClient(logger, client.KubernetesClient())

	return &Prepare{
		client:    client,
		logger:    logger,
		dnsClient: dnsClient,
	}
}

// StartInformers checks if the required informers can start and sync in a reasonable time.
func (p *Prepare) StartInformers(acl bool) error {
	stopCh := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := p.startBaseInformers(ctx, stopCh); err != nil {
		return err
	}

	if !acl {
		return nil
	}

	if err := p.startACLInformers(ctx, stopCh); err != nil {
		return err
	}

	return nil
}

func (p *Prepare) startBaseInformers(ctx context.Context, stopCh <-chan struct{}) error {
	// Create a new SharedInformerFactory, and register the event handler to informers.
	kubeFactory := informers.NewSharedInformerFactoryWithOptions(p.client.KubernetesClient(), k8s.ResyncPeriod)
	kubeFactory.Core().V1().Services().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	kubeFactory.Core().V1().Endpoints().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	kubeFactory.Start(stopCh)

	for t, ok := range kubeFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	splitFactory := splitinformer.NewSharedInformerFactoryWithOptions(p.client.SplitClient(), k8s.ResyncPeriod)
	splitFactory.Split().V1alpha2().TrafficSplits().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	splitFactory.Start(stopCh)

	for t, ok := range splitFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	return nil
}

func (p *Prepare) startACLInformers(ctx context.Context, stopCh <-chan struct{}) error {
	// Create new SharedInformerFactories, and register the event handler to informers.
	accessFactory := accessinformer.NewSharedInformerFactoryWithOptions(p.client.AccessClient(), k8s.ResyncPeriod)
	accessFactory.Access().V1alpha1().TrafficTargets().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	accessFactory.Start(stopCh)

	for t, ok := range accessFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	specsFactory := specsinformer.NewSharedInformerFactoryWithOptions(p.client.SpecsClient(), k8s.ResyncPeriod)
	specsFactory.Specs().V1alpha1().HTTPRouteGroups().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	specsFactory.Specs().V1alpha1().TCPRoutes().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	specsFactory.Start(stopCh)

	for t, ok := range specsFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	// Create a new SharedInformerFactory, and register the event handler to informers.
	kubeFactory := informers.NewSharedInformerFactoryWithOptions(p.client.KubernetesClient(), k8s.ResyncPeriod)
	kubeFactory.Core().V1().Pods().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	kubeFactory.Start(stopCh)

	for t, ok := range kubeFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for controller caches to sync: %s", t.String())
		}
	}

	return nil
}

// CheckDNSProvider checks if the required informers can start and sync in a reasonable time.
func (p *Prepare) CheckDNSProvider(ctx context.Context) (dns.Provider, error) {
	return p.dnsClient.CheckDNSProvider(ctx)
}

// ConfigureCoreDNS patches the CoreDNS configuration for Maesh.
func (p *Prepare) ConfigureCoreDNS(ctx context.Context, coreDNSNamespace, clusterDomain, maeshNamespace string) error {
	return p.dnsClient.ConfigureCoreDNS(ctx, coreDNSNamespace, clusterDomain, maeshNamespace)
}

// ConfigureKubeDNS patches the KubeDNS configuration for Maesh.
func (p *Prepare) ConfigureKubeDNS(ctx context.Context, clusterDomain, maeshNamespace string) error {
	return p.dnsClient.ConfigureKubeDNS(ctx, clusterDomain, maeshNamespace)
}

// ConfigureDNS configures and patches the DNS system.
func (p *Prepare) ConfigureDNS(ctx context.Context, clusterDomain, maeshNamespace string) error {
	provider, err := p.CheckDNSProvider(ctx)
	if err != nil {
		return fmt.Errorf("error during cluster check: %v", err)
	}

	switch provider {
	case dns.CoreDNS:
		if err := p.ConfigureCoreDNS(ctx, metav1.NamespaceSystem, clusterDomain, maeshNamespace); err != nil {
			return fmt.Errorf("unable to configure CoreDNS: %v", err)
		}
	case dns.KubeDNS:
		if err := p.ConfigureKubeDNS(ctx, clusterDomain, maeshNamespace); err != nil {
			return fmt.Errorf("unable to configure KubeDNS: %v", err)
		}
	}

	return nil
}
