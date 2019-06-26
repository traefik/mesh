package smi

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/containous/traefik/pkg/config"
	"github.com/containous/traefik/pkg/job"
	"github.com/containous/traefik/pkg/log"
	"github.com/containous/traefik/pkg/provider/kubernetes/crd/traefik/v1alpha1"
	"github.com/containous/traefik/pkg/safe"
	accessv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/access/v1alpha1"
	specsv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/specs/v1alpha1"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Provider holds configurations of the provider.
type Provider struct {
	Endpoint          string   `description:"Kubernetes server endpoint (required for external cluster client)."`
	Token             string   `description:"Kubernetes bearer token (not needed for in-cluster client)."`
	CertAuthFilePath  string   `description:"Kubernetes certificate authority file path (not needed for in-cluster client)."`
	Namespaces        []string `description:"Kubernetes namespaces." export:"true"`
	LabelSelector     string   `description:"Kubernetes label selector to use." export:"true"`
	lastConfiguration safe.Safe
}

// destinationKey is used to key a grouped map of trafficTargets.
type destinationKey struct {
	name      string
	namespace string
	port      string
}

func (p *Provider) newK8sClient(ctx context.Context, labelSelector string) (*clientWrapper, error) {
	labelSel, err := labels.Parse(labelSelector)
	if err != nil {
		return nil, fmt.Errorf("invalid label selector: %q", labelSelector)
	}
	log.FromContext(ctx).Infof("label selector is: %q", labelSel)

	withEndpoint := ""
	if p.Endpoint != "" {
		withEndpoint = fmt.Sprintf(" with endpoint %v", p.Endpoint)
	}

	var client *clientWrapper
	switch {
	case os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "":
		log.FromContext(ctx).Infof("Creating in-cluster Provider client%s", withEndpoint)
		client, err = newInClusterClient(p.Endpoint)
	case os.Getenv("KUBECONFIG") != "":
		log.FromContext(ctx).Infof("Creating cluster-external Provider client from KUBECONFIG %s", os.Getenv("KUBECONFIG"))
		client, err = newExternalClusterClientFromFile(os.Getenv("KUBECONFIG"))
	default:
		log.FromContext(ctx).Infof("Creating cluster-external Provider client%s", withEndpoint)
		client, err = newExternalClusterClient(p.Endpoint, p.Token, p.CertAuthFilePath)
	}

	if err == nil {
		client.labelSelector = labelSel
	}

	return client, err
}

// Init the provider.
func (p *Provider) Init() error {
	return nil
}

// Provide allows the i3o provider to provide configurations to traefik
// using the given configuration channel based on services and SMI objects.
func (p *Provider) Provide(configurationChan chan<- config.Message, pool *safe.Pool) error {
	ctxLog := log.With(context.Background(), log.Str(log.ProviderName, "i3o"))
	logger := log.FromContext(ctxLog)
	// Tell glog (used by client-go) to log into STDERR. Otherwise, we risk
	// certain kinds of API errors getting logged into a directory not
	// available in a `FROM scratch` Docker container, causing glog to abort
	// hard with an exit code > 0.
	err := flag.Set("logtostderr", "true")
	if err != nil {
		return err
	}

	logger.Debugf("Using label selector: %q", p.LabelSelector)
	k8sClient, err := p.newK8sClient(ctxLog, p.LabelSelector)
	if err != nil {
		return err
	}

	pool.Go(func(stop chan bool) {
		operation := func() error {
			stopWatch := make(chan struct{}, 1)
			defer close(stopWatch)
			eventsChan, err := k8sClient.WatchAll(p.Namespaces, stopWatch)
			if err != nil {
				logger.Errorf("Error watching kubernetes events: %v", err)
				timer := time.NewTimer(1 * time.Second)
				select {
				case <-timer.C:
					return err
				case <-stop:
					return nil
				}
			}
			for {
				select {
				case <-stop:
					return nil
				case event := <-eventsChan:
					conf := p.loadConfiguration(ctxLog, k8sClient)

					if reflect.DeepEqual(p.lastConfiguration.Get(), conf) {
						logger.Debugf("Skipping Kubernetes event kind %T", event)
					} else {
						p.lastConfiguration.Set(conf)
						configurationChan <- config.Message{
							ProviderName:  "i3o",
							Configuration: conf,
						}
					}
				}
			}
		}

		notify := func(err error, time time.Duration) {
			logger.Errorf("Provider connection error: %s; retrying in %s", err, time)
		}
		err := backoff.RetryNotify(safe.OperationWithRecover(operation), job.NewBackOff(backoff.NewExponentialBackOff()), notify)
		if err != nil {
			logger.Errorf("Cannot connect to Provider: %s", err)
		}
	})

	return nil
}

func checkStringQuoteValidity(value string) error {
	_, err := strconv.Unquote(`"` + value + `"`)
	return err
}

func loadTCPServers(client Client, namespace string, svc v1alpha1.ServiceTCP, serviceAccountName string) ([]config.TCPServer, error) {
	service, exists, err := client.GetService(namespace, svc.Name)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, errors.New("service not found")
	}

	var portSpec *corev1.ServicePort
	for _, p := range service.Spec.Ports {
		if svc.Port == p.Port {
			portSpec = &p
			break
		}
	}

	if portSpec == nil {
		return nil, errors.New("service port not found")
	}

	var servers []config.TCPServer
	if service.Spec.Type == corev1.ServiceTypeExternalName {
		servers = append(servers, config.TCPServer{
			Address: fmt.Sprintf("%s:%d", service.Spec.ExternalName, portSpec.Port),
		})
	} else {
		endpoints, endpointsExists, endpointsErr := client.GetEndpoints(namespace, svc.Name)
		if endpointsErr != nil {
			return nil, endpointsErr
		}

		if !endpointsExists {
			return nil, errors.New("endpoints not found")
		}

		if len(endpoints.Subsets) == 0 {
			return nil, errors.New("subset not found")
		}

		var port int32
		for _, subset := range endpoints.Subsets {
			for _, p := range subset.Ports {
				if portSpec.Name == p.Name {
					port = p.Port
					break
				}
			}

			if port == 0 {
				return nil, errors.New("cannot define a port")
			}

			for _, addr := range subset.Addresses {
				if serviceAccountName != "" {
					pod, exists, err := client.GetPod(addr.TargetRef.Namespace, addr.TargetRef.Name)
					if err != nil {
						return nil, err
					}
					if !exists {
						continue
					}
					if pod.Spec.ServiceAccountName != serviceAccountName {
						continue
					}
				}
				servers = append(servers, config.TCPServer{
					Address: fmt.Sprintf("%s:%d", addr.IP, port),
				})
			}
		}
	}

	return servers, nil
}

func loadServers(client Client, namespace string, svc v1alpha1.Service, serviceAccountName string) ([]config.Server, error) {
	strategy := svc.Strategy
	if strategy == "" {
		strategy = "RoundRobin"
	}
	if strategy != "RoundRobin" {
		return nil, fmt.Errorf("load balancing strategy %v is not supported", strategy)
	}

	service, exists, err := client.GetService(namespace, svc.Name)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, errors.New("service not found")
	}

	var portSpec *corev1.ServicePort
	for _, p := range service.Spec.Ports {
		if svc.Port == p.Port {
			portSpec = &p
			break
		}
	}

	if portSpec == nil {
		return nil, errors.New("service port not found")
	}

	var servers []config.Server
	if service.Spec.Type == corev1.ServiceTypeExternalName {
		servers = append(servers, config.Server{
			URL: fmt.Sprintf("http://%s:%d", service.Spec.ExternalName, portSpec.Port),
		})
	} else {
		endpoints, endpointsExists, endpointsErr := client.GetEndpoints(namespace, svc.Name)
		if endpointsErr != nil {
			return nil, endpointsErr
		}

		if !endpointsExists {
			return nil, errors.New("endpoints not found")
		}

		if len(endpoints.Subsets) == 0 {
			return nil, errors.New("subset not found")
		}

		var port int32
		for _, subset := range endpoints.Subsets {
			for _, p := range subset.Ports {
				if portSpec.Name == p.Name {
					port = p.Port
					break
				}
			}

			if port == 0 {
				return nil, errors.New("cannot define a port")
			}

			protocol := "http"
			if port == 443 || strings.HasPrefix(portSpec.Name, "https") {
				protocol = "https"
			}

			for _, addr := range subset.Addresses {
				if serviceAccountName != "" {
					pod, exists, err := client.GetPod(addr.TargetRef.Namespace, addr.TargetRef.Name)
					if err != nil {
						return nil, err
					}
					if !exists {
						continue
					}
					if pod.Spec.ServiceAccountName != serviceAccountName {
						continue
					}
				}
				servers = append(servers, config.Server{
					URL: fmt.Sprintf("%s://%s:%d", protocol, addr.IP, port),
				})
			}
		}
	}

	return servers, nil
}

func (p *Provider) loadConfiguration(ctx context.Context, client Client) *config.Configuration {
	configRouters := make(map[string]*config.Router)
	configServices := make(map[string]*config.Service)
	namespaces := client.GetNamespaces()

	for _, namespace := range namespaces {
		trafficTargets := client.GetTrafficTargetsWithDestinationInNamespace(namespace.Name)

		services := client.GetServicesInNamespace(namespace.Name)

		for _, service := range services {
			applicableTrafficTargets := p.getApplicableTrafficTargets(service, trafficTargets, client)

			groupedByDestinationTrafficTargets := p.groupTrafficTargetsByDestination(applicableTrafficTargets)

			for _, groupedTrafficTargets := range groupedByDestinationTrafficTargets {
				for _, groupedTrafficTarget := range groupedTrafficTargets {
					key := uuid.New().String()
					configRouters[key] = p.buildRouterFromTrafficTarget(service, groupedTrafficTarget, client)
					configServices[key] = p.buildServiceFromTrafficTarget(service, groupedTrafficTarget, client)
				}
			}

		}
	}

	return &config.Configuration{
		HTTP: &config.HTTPConfiguration{
			Routers:  configRouters,
			Services: configServices,
		},
	}
}

func (p *Provider) getApplicableTrafficTargets(service *corev1.Service, trafficTargets []*accessv1alpha1.TrafficTarget, client Client) []*accessv1alpha1.TrafficTarget {
	var result []*accessv1alpha1.TrafficTarget

	endpoint, exists, err := client.GetEndpoints(service.Namespace, service.Name)
	if err != nil {
		log.Errorf("Could not get endpoints for service %s/%s: %v", service.Namespace, service.Name, err)
		return nil
	}
	if !exists {
		log.Errorf("Endpoints for service %s/%s do not exist", service.Namespace, service.Name)
		return nil
	}

	for _, subset := range endpoint.Subsets {
		for _, trafficTarget := range trafficTargets {
			if service.Namespace != trafficTarget.Destination.Namespace {
				// Destination not in service namespace, skip.
				continue
			}

			var subsetMatch bool
			for _, endpointPort := range subset.Ports {
				if strconv.FormatInt(int64(endpointPort.Port), 10) == trafficTarget.Destination.Port {
					subsetMatch = true
					break
				}
			}

			if !subsetMatch {
				// No subset port match on destination port, so subset is not affected
				continue
			}

			var validPodFound bool
			for _, address := range subset.Addresses {
				if pod, _, err := client.GetPod(address.TargetRef.Namespace, address.TargetRef.Name); err != nil {
					if pod.Spec.ServiceAccountName == trafficTarget.Destination.Name {
						validPodFound = true
						break
					}
				}
			}

			if !validPodFound {
				// No valid pods with serviceAccound found on the subset, so it is not affected
				continue
			}

			// We have a subset match, and valid referenced pods for the trafficTarget.
			result = append(result, trafficTarget)
		}

	}

	return result
}

func (p *Provider) groupTrafficTargetsByDestination(trafficTargets []*accessv1alpha1.TrafficTarget) map[destinationKey][]*accessv1alpha1.TrafficTarget {
	result := make(map[destinationKey][]*accessv1alpha1.TrafficTarget)

	for _, trafficTarget := range trafficTargets {
		key := destinationKey{
			name:      trafficTarget.Destination.Name,
			namespace: trafficTarget.Destination.Namespace,
			port:      trafficTarget.Destination.Port,
		}

		if _, ok := result[key]; !ok {
			// If the destination key does not exist, create the key.
			result[key] = []*accessv1alpha1.TrafficTarget{}
		}

		result[key] = append(result[key], trafficTarget)
	}

	return result
}

func (p *Provider) buildRouterFromTrafficTarget(service *corev1.Service, trafficTarget *accessv1alpha1.TrafficTarget, client Client) *config.Router {
	var result *config.Router
	var rule []string
	for _, spec := range trafficTarget.Specs {
		if spec.Kind != "HTTPRouteGroup" {
			// TCP is unsupported for now.
			continue
		}
		var builtRule []string
		rawHTTPRouteGroup, _, err := client.GetHTTPRouteGroup(trafficTarget.Namespace, spec.Name)
		if err != nil {
			log.Errorf("Error getting HTTPRouteGroup: %v", err)
			continue
		}

		for _, match := range spec.Matches {
			for _, httpMatch := range rawHTTPRouteGroup.Matches {
				if match != httpMatch.Name {
					// Matches specified, add only matches from route group
					continue
				}
				builtRule = append(builtRule, p.buildRuleSnippetFromMatch(httpMatch))
			}
		}
		rule = append(rule, "("+strings.Join(builtRule, " || ")+")")
	}

	result.Rule = strings.Join(rule, " || ")
	return result
}

func (p *Provider) buildRuleSnippetFromMatch(match specsv1alpha1.HTTPMatch) string {
	var result []string
	if len(match.PathRegex) > 0 {
		result = append(result, fmt.Sprintf("PathPrefix(`%s`)", match.PathRegex))
	}

	if len(match.Methods) > 0 {
		methods := strings.Join(match.Methods, ",")
		result = append(result, fmt.Sprintf("Methods(%s)", methods))
	}

	return "(" + strings.Join(result, " && ") + ")"
}

func (p *Provider) buildServiceFromTrafficTarget(service *corev1.Service, trafficTarget *accessv1alpha1.TrafficTarget, client Client) *config.Service {
	var servers []config.Server

	if service.Namespace != trafficTarget.Destination.Namespace {
		// Destination not in service namespace log error.
		log.Errorf("TrafficTarget %s/%s destination not in namespace %s", trafficTarget.Namespace, trafficTarget.Name, service.Namespace)
		return nil
	}

	endpoint, exists, err := client.GetEndpoints(service.Namespace, service.Name)
	if err != nil {
		log.Errorf("Could not get endpoints for service %s/%s: %v", service.Namespace, service.Name, err)
		return nil
	}
	if !exists {
		log.Errorf("Endpoints for service %s/%s do not exist", service.Namespace, service.Name)
		return nil
	}

	for _, subset := range endpoint.Subsets {
		var subsetMatch bool
		for _, endpointPort := range subset.Ports {
			if strconv.FormatInt(int64(endpointPort.Port), 10) == trafficTarget.Destination.Port {
				subsetMatch = true
				break
			}
		}

		if !subsetMatch {
			// No subset port match on destination port, so subset is not affected
			continue
		}

		for _, address := range subset.Addresses {
			if pod, _, err := client.GetPod(address.TargetRef.Namespace, address.TargetRef.Name); err != nil {
				if pod.Spec.ServiceAccountName == trafficTarget.Destination.Name {
					server := config.Server{
						URL: "http://" + net.JoinHostPort(address.IP, trafficTarget.Destination.Port),
					}
					servers = append(servers, server)
				}
			}
		}
	}

	lb := &config.LoadBalancerService{
		PassHostHeader: true,
		Servers:        servers,
	}

	return &config.Service{
		LoadBalancer: lb,
	}
}

func makeServiceKey(rule, ingressName string) (string, error) {
	h := sha256.New()
	if _, err := h.Write([]byte(rule)); err != nil {
		return "", err
	}

	ingressName = strings.ReplaceAll(ingressName, ".", "-")
	key := fmt.Sprintf("%s-%.10x", ingressName, h.Sum(nil))

	return key, nil
}

func makeID(namespace, name string) string {
	if namespace == "" {
		return name
	}

	return namespace + "/" + name
}
