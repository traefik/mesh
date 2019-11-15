package resource

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

// Maesh resources labels.
const (
	AppLabel       = "app"
	ComponentLabel = "component"

	AppLabelMaesh          = "maesh"
	ComponentLabelMeshSvc  = "mesh-svc"
	ComponentLabelMeshNode = "mesh-node"
)

// MeshServiceLabels returns the labels to apply to a mesh service
func MeshServiceLabels() map[string]string {
	return map[string]string{
		AppLabel:       AppLabelMaesh,
		ComponentLabel: ComponentLabelMeshSvc,
	}
}

// MeshServiceSelector returns the selectors carried by a mesh service.
func MeshServiceSelector() map[string]string {
	return map[string]string{
		AppLabel:       AppLabelMaesh,
		ComponentLabel: ComponentLabelMeshNode,
	}
}

// IsMeshService returns true if a service is a mesh service.
func IsMeshService(obj interface{}) (bool, error) {
	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return false, err
	}

	objLabels := objMeta.GetLabels()

	isMaeshResource := objLabels[AppLabel] == AppLabelMaesh
	isMeshSvc := objLabels[ComponentLabel] == ComponentLabelMeshSvc

	return isMaeshResource && isMeshSvc, nil
}

// IsMeshPod returns true if an object is a mesh pod.
func IsMeshPod(obj interface{}) (bool, error) {
	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return false, err
	}

	objLabels := objMeta.GetLabels()

	isMaeshResource := objLabels[AppLabel] == AppLabelMaesh
	isMeshNode := objLabels[ComponentLabel] == ComponentLabelMeshNode

	return isMaeshResource && isMeshNode, nil
}

// MeshPodsLabelsSelector returns the selector to apply when looking for maesh pods.
func MeshPodsLabelsSelector() labels.Selector {
	sel := labels.Everything()
	sel = sel.Add(mustRequirement(labels.NewRequirement(AppLabel, selection.Equals, []string{AppLabelMaesh})))
	sel = sel.Add(mustRequirement(labels.NewRequirement(ComponentLabel, selection.Equals, []string{ComponentLabelMeshNode})))
	return sel
}

func mustRequirement(c *labels.Requirement, err error) labels.Requirement {
	if err != nil {
		panic(err)
	}

	return *c
}
