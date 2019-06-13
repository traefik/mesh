package k8s

const (
	MeshNamespace string = "traefik-mesh"
)

// Namespaces holds namespace name.
type Namespaces []string

// Contains returns true if x is in the slice, false otherwise.
func (n Namespaces) Contains(x string) bool {
	for _, v := range n {
		if x == v {
			return true
		}
	}
	return false
}
