package k8s

import (
	"time"
)

const (
	ResyncPeriod                 = 5 * time.Minute
	baseAnnotation        string = "maesh.containo.us/"
	AnnotationServiceType        = baseAnnotation + "maesh-traffic-type"
	ServiceTypeHTTP       string = "http"
	ServiceTypeTCP        string = "tcp"
	BlockAllMiddlewareKey string = "smi-block-all-middleware"
)
