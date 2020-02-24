package k8s

// Namespaces holds namespace name.
type Namespaces []string

// Contains returns true if x is in the slice, false otherwise.
func (n Namespaces) Contains(x string) bool {
	return contains(n, x)
}

func contains(s []string, x string) bool {
	for _, v := range s {
		if x == v {
			return true
		}
	}

	return false
}
