package k8s

// ServicePort holds a combination of service namespace, name and port.
type ServicePort struct {
	Namespace string
	Name      string
	Port      int32
}

// Service holds a combination of service namespace and name.
type Service struct {
	Namespace string
	Name      string
}

// Services holds a list of type Service.
type Services []Service

// Contains returns true if a service with matching name and namespace is in the slice, false otherwise.
func (s Services) Contains(name, namespace string) bool {
	for _, v := range s {
		if name == v.Name && namespace == v.Namespace {
			return true
		}
	}

	return false
}
