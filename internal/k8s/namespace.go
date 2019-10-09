package k8s

import (
	"strings"
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

// ObjectKeyInNamespace returns true if the object key is in the namespace.
func ObjectKeyInNamespace(key string, namespaces Namespaces) bool {
	splitKey := strings.Split(key, "/")
	if len(splitKey) == 1 {
		// No namespace in the key
		return false
	}

	return namespaces.Contains(splitKey[0])
}
