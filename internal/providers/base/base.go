package base

import (
	"github.com/containous/maesh/internal/k8s"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	splitv1alpha1 "github.com/deislabs/smi-sdk-go/pkg/apis/split/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// CreateBaseConfigWithReadiness creates a base configuration for deploying to mesh nodes.
func CreateBaseConfigWithReadiness() *dynamic.Configuration {
	return &dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{
			Routers: map[string]*dynamic.Router{
				"readiness": {
					Rule:        "Path(`/ping`)",
					EntryPoints: []string{"readiness"},
					Service:     "readiness",
				},
			},
			Services: map[string]*dynamic.Service{
				"readiness": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						Servers: []dynamic.Server{
							{
								URL: "http://127.0.0.1:8080",
							},
						},
					},
				},
			},
			Middlewares: map[string]*dynamic.Middleware{},
		},
		TCP: &dynamic.TCPConfiguration{
			Routers:  map[string]*dynamic.TCPRouter{},
			Services: map[string]*dynamic.TCPService{},
		},
	}
}

// GetTrafficSplitFromList returns a trafficsplit from a list.
func GetTrafficSplitFromList(serviceName string, trafficSplits []*splitv1alpha1.TrafficSplit) *splitv1alpha1.TrafficSplit {
	for _, t := range trafficSplits {
		if t.Spec.Service == serviceName {
			return t
		}
	}

	return nil
}

// GetEndpointsFromList returns an endpoint from a list.
func GetEndpointsFromList(name, namespace string, endpointList []*corev1.Endpoints) *corev1.Endpoints {
	for _, endpoints := range endpointList {
		if endpoints.Name == name && endpoints.Namespace == namespace {
			return endpoints
		}
	}

	return nil
}

// AddBaseSMIMiddlewares adds base middleware to a dynamic config.
func AddBaseSMIMiddlewares(config *dynamic.Configuration) {
	blockAll := &dynamic.Middleware{
		IPWhiteList: &dynamic.IPWhiteList{
			SourceRange: []string{"255.255.255.255"},
		},
	}

	config.HTTP.Middlewares[k8s.BlockAllMiddlewareKey] = blockAll
}
