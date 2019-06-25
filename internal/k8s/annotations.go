package k8s

const (
	baseAnnotationURL          = "i3o.containo.us/"
	AnnotationServiceType      = baseAnnotationURL + "i3o-traffic-type"
	ServiceTypeHTTP            = "http"
	AnnotationSMIDestinationSA = baseAnnotationURL + "i3o-smi-destination-sa"
)
