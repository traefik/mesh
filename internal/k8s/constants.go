package k8s

import (
	"time"
)

const (
	ResyncPeriod                 = 5 * time.Minute
	baseAnnotation        string = "i3o.containo.us/"
	AnnotationServiceType        = baseAnnotation + "i3o-traffic-type"
	ServiceTypeHTTP       string = "http"
	ServiceTypeTCP        string = "tcp"
	BlockAllMiddlewareKey string = "smi-block-all-middleware"
)
