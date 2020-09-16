package provider

import (
	"fmt"

	"github.com/traefik/mesh/pkg/topology"
)

const (
	blockAllMiddlewareKey = "block-all-middleware"
	blockAllServiceKey    = "block-all-service"
)

func getMiddlewareKey(svc *topology.Service, name string) string {
	return fmt.Sprintf("%s-%s-%s", svc.Namespace, svc.Name, name)
}

func getServiceRouterKeyFromService(svc *topology.Service, port int32) string {
	return fmt.Sprintf("%s-%s-%d", svc.Namespace, svc.Name, port)
}

func getWhitelistMiddlewareKeyFromTrafficTargetDirect(tt *topology.ServiceTrafficTarget) string {
	return fmt.Sprintf("%s-%s-%s-whitelist-traffic-target-direct", tt.Service.Namespace, tt.Service.Name, tt.Name)
}

func getWhitelistMiddlewareKeyFromTrafficTargetIndirect(tt *topology.ServiceTrafficTarget) string {
	return fmt.Sprintf("%s-%s-%s-whitelist-traffic-target-indirect", tt.Service.Namespace, tt.Service.Name, tt.Name)
}

func getWhitelistMiddlewareKeyFromTrafficSplitDirect(ts *topology.TrafficSplit) string {
	return fmt.Sprintf("%s-%s-%s-whitelist-traffic-split-direct", ts.Service.Namespace, ts.Service.Name, ts.Name)
}

func getWhitelistMiddlewareKeyFromTrafficSplitIndirect(ts *topology.TrafficSplit) string {
	return fmt.Sprintf("%s-%s-%s-whitelist-traffic-split-indirect", ts.Service.Namespace, ts.Service.Name, ts.Name)
}

func getServiceKeyFromTrafficTarget(tt *topology.ServiceTrafficTarget, port int32) string {
	return fmt.Sprintf("%s-%s-%s-%d-traffic-target", tt.Service.Namespace, tt.Service.Name, tt.Name, port)
}

func getRouterKeyFromTrafficTargetDirect(tt *topology.ServiceTrafficTarget, port int32) string {
	return fmt.Sprintf("%s-%s-%s-%d-traffic-target-direct", tt.Service.Namespace, tt.Service.Name, tt.Name, port)
}

func getRouterKeyFromTrafficTargetIndirect(tt *topology.ServiceTrafficTarget, port int32) string {
	return fmt.Sprintf("%s-%s-%s-%d-traffic-target-indirect", tt.Service.Namespace, tt.Service.Name, tt.Name, port)
}

func getServiceKeyFromTrafficSplit(ts *topology.TrafficSplit, port int32) string {
	return fmt.Sprintf("%s-%s-%s-%d-traffic-split", ts.Service.Namespace, ts.Service.Name, ts.Name, port)
}

func getRouterKeyFromTrafficSplitDirect(ts *topology.TrafficSplit, port int32) string {
	return fmt.Sprintf("%s-%s-%s-%d-traffic-split-direct", ts.Service.Namespace, ts.Service.Name, ts.Name, port)
}

func getRouterKeyFromTrafficSplitIndirect(ts *topology.TrafficSplit, port int32) string {
	return fmt.Sprintf("%s-%s-%s-%d-traffic-split-indirect", ts.Service.Namespace, ts.Service.Name, ts.Name, port)
}

func getServiceKeyFromTrafficSplitBackend(ts *topology.TrafficSplit, port int32, backend topology.TrafficSplitBackend) string {
	return fmt.Sprintf("%s-%s-%s-%d-%s-traffic-split-backend", ts.Service.Namespace, ts.Service.Name, ts.Name, port, backend.Service.Name)
}
