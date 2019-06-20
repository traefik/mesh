package try

import (
	"fmt"
	"strings"

	traefikv1alpha1 "github.com/containous/traefik/pkg/provider/kubernetes/crd/traefik/v1alpha1"
)

type List []string

func (l List) contains(s string) bool {
	for _, v := range l {
		if strings.Contains(s, v) {
			return true
		}
	}
	return false
}

type IngressRouteListCondition func(*traefikv1alpha1.IngressRouteList) error

func HasIngressRouteList(nb int) IngressRouteListCondition {
	return func(lst *traefikv1alpha1.IngressRouteList) error {
		if len(lst.Items) != nb {
			return fmt.Errorf("unable to find %d ingressroutes in %v", nb, lst.Items)
		}
		return nil
	}
}

func HasNamesIngressRouteList(names List) IngressRouteListCondition {
	return func(lst *traefikv1alpha1.IngressRouteList) error {
		for _, value := range lst.Items {
			if !names.contains(value.Name) {
				return fmt.Errorf("unable to find %q in %v", value.Name, names)
			}
		}
		return nil
	}
}

type IngressRouteTCPListCondition func(list *traefikv1alpha1.IngressRouteTCPList) error

func HasIngressRouteTCPList(nb int) IngressRouteTCPListCondition {
	return func(lst *traefikv1alpha1.IngressRouteTCPList) error {
		if len(lst.Items) != nb {
			return fmt.Errorf("unable to find %d ingressroutetcps in %v", nb, lst.Items)
		}
		return nil
	}
}

func HasNamesIngressRouteTCPList(names List) IngressRouteTCPListCondition {
	return func(lst *traefikv1alpha1.IngressRouteTCPList) error {
		for _, value := range lst.Items {
			if !names.contains(value.Name) {
				return fmt.Errorf("unable to find %q in %v", value.Name, names)
			}
		}
		return nil
	}
}
