package dns

import (
	"fmt"
	"net"

	"github.com/miekg/dns"
	"github.com/traefik/mesh/v2/pkg/controller"
	"github.com/traefik/mesh/v2/pkg/k8s"
	listers "k8s.io/client-go/listers/core/v1"
)

// ShadowServiceResolver is a DNS resolver implementation which resolves the shadow service ClusterIP corresponding to a subdomain.
// The subdomain must be of form: name.namespace.domain where name and namespace match the shadowed service metadata and domain the configured domain.
type ShadowServiceResolver struct {
	domain        string
	namespace     string
	serviceLister listers.ServiceLister
}

// NewShadowServiceResolver creates and returns a new resolver.
func NewShadowServiceResolver(domain, namespace string, serviceLister listers.ServiceLister) *ShadowServiceResolver {
	return &ShadowServiceResolver{
		domain:        domain,
		namespace:     namespace,
		serviceLister: serviceLister,
	}
}

// Domain returns the configured domain.
func (r *ShadowServiceResolver) Domain() string {
	return r.domain
}

// LookupFQDN returns the ClusterIP of the Shadow Service corresponding to the given FQDN.
func (r *ShadowServiceResolver) LookupFQDN(fqdn string) (net.IP, error) {
	namespace, name, err := r.parseNamespaceAndName(fqdn)
	if err != nil {
		return nil, err
	}

	shadowServiceName, err := controller.GetShadowServiceName(namespace, name)
	if err != nil {
		return nil, err
	}

	shadowService, err := r.serviceLister.Services(r.namespace).Get(shadowServiceName)
	if err != nil {
		return nil, fmt.Errorf("unable to get shadow service %q: %w", shadowServiceName, err)
	}

	if shadowService.Labels[k8s.LabelServiceNamespace] != namespace || shadowService.Labels[k8s.LabelServiceName] != name {
		return nil, fmt.Errorf("service labels in %q does not match service name %q and namespace %q", shadowServiceName, name, namespace)
	}

	return net.ParseIP(shadowService.Spec.ClusterIP), nil
}

// parseNamespaceAndName returns the namespace and the name corresponding to the given FQDN.
func (r *ShadowServiceResolver) parseNamespaceAndName(fqdn string) (string, string, error) {
	domain := dns.CanonicalName(r.domain)

	if !dns.IsSubDomain(domain, fqdn) {
		return "", "", fmt.Errorf("name %q is not a subdomain of %q", fqdn, domain)
	}

	labels := dns.SplitDomainName(fqdn)
	if len(labels)-dns.CountLabel(domain) < 2 {
		return "", "", fmt.Errorf("malformed name %q", fqdn)
	}

	return labels[1], labels[0], nil
}
