package controller

import (
	"testing"

	"github.com/containous/maesh/internal/k8s"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const meshNamespace string = "maesh"

func TestNewController(t *testing.T) {
	clientMock := k8s.NewClientMock("mock.yaml")

	// Test basic create/init.
	controller := NewMeshController(clientMock, false, "http", meshNamespace, []string{}, 9000)
	assert.NotNil(t, controller)

	// Test SMI create/init.
	controller = NewMeshController(clientMock, true, "http", meshNamespace, []string{}, 9000)
	assert.NotNil(t, controller)

	// Test list of ignored namespaces load.
	controller = NewMeshController(clientMock, true, "http", meshNamespace, []string{"foo", "bar"}, 9000)
	assert.NotNil(t, controller)
	assert.Equal(t, []string{"foo", "bar", metav1.NamespaceSystem}, controller.ignored.GetIgnoredNamespaces())
}
