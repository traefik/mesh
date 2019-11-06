package k8s

// IgnoreWrapper holds namespaces and services to ignore.
type IgnoreWrapper struct {
	Namespaces    Namespaces
	Services      Services
	MeshNamespace string
}

// NewIgnored returns a new IgnoreWrapper.
func NewIgnored() IgnoreWrapper {
	return IgnoreWrapper{
		Namespaces:    Namespaces{},
		Services:      Services{},
		MeshNamespace: "",
	}
}

// SetMeshNamespace sets the meshNamespace.
func (i *IgnoreWrapper) SetMeshNamespace(namespace string) {
	i.MeshNamespace = namespace
}

// AddIgnoredNamespace adds a namespace to the list of ignored namespaces.
func (i *IgnoreWrapper) AddIgnoredNamespace(namespace string) {
	i.Namespaces = append(i.Namespaces, namespace)
}

// GetIgnoredNamespaces gets a list of ignored namespaces.
func (i *IgnoreWrapper) GetIgnoredNamespaces() []string {
	return i.Namespaces
}

// AddIgnoredService adds a service to the list of ignored services.
func (i *IgnoreWrapper) AddIgnoredService(serviceName, serviceNamespace string) {
	i.Services = append(i.Services, Service{Name: serviceName, Namespace: serviceNamespace})
}

// IsIgnoredService returns if the service's events should be ignored.
func (i *IgnoreWrapper) IsIgnoredService(name, namespace string) bool {
	// Is the service's namespace ignored?
	if i.Namespaces.Contains(namespace) {
		return true
	}

	// Is the service explicitly ignored?
	if i.Services.Contains(name, namespace) {
		return true
	}

	// Is the service in the mesh namespace?
	if i.MeshNamespace != "" && namespace == i.MeshNamespace {
		return true
	}

	return false
}

// IsIgnoredNamespace returns if the service's events should be ignored.
func (i *IgnoreWrapper) IsIgnoredNamespace(namespace string) bool {
	// Is the namespace ignored?
	if i.Namespaces.Contains(namespace) {
		return true
	}

	// Is the namespace the mesh namespace?
	if i.MeshNamespace != "" && namespace == i.MeshNamespace {
		return true
	}

	return false
}
