package utils

import (
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

const (
	MeshNamespace string = "traefik-mesh"
)

// InitCluster is used to initialize a kubernetes cluster with a variety of configuration options.
func InitCluster(client kubernetes.Interface) error {
	log.Infoln("Preparing Cluster...")
	defer log.Infoln("Cluster Preparation Complete...")

	log.Debugln("Creating mesh namespace...")
	if err := verifyNamespaceExists(client, MeshNamespace); err != nil {
		return err
	}

	log.Debugln("Patching CoreDNS...")
	if err := patchCoreDNS(client, "coredns", metav1.NamespaceSystem); err != nil {
		return err
	}

	return nil
}

// VerifyCluster is used to verify a kubernetes cluster has been initialized properly.
func VerifyCluster(client kubernetes.Interface) error {
	log.Infoln("Verifying Cluster...")
	defer log.Infoln("Cluster Verification Complete...")

	log.Debugln("Verifying mesh namespace exists...")
	if err := verifyNamespaceExists(client, MeshNamespace); err != nil {
		return err
	}

	log.Debugln("Verifying CoreDNS Patched...")
	if err := verifyCoreDNSPatched(client, "coredns", metav1.NamespaceSystem); err != nil {
		return err
	}

	return nil
}

func verifyNamespaceExists(client kubernetes.Interface, namespace string) error {
	_, err := client.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err != nil {
		ns := &apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
			Spec: apiv1.NamespaceSpec{},
		}

		_, err := client.CoreV1().Namespaces().Create(ns)
		if err != nil {
			return err
		}

	}
	return nil
}

func verifyCoreDNSPatched(client kubernetes.Interface, deploymentName, deploymentNamespace string) error {
	coreDeployment, err := client.AppsV1().Deployments(deploymentNamespace).Get(deploymentName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if len(coreDeployment.Spec.Template.Spec.Volumes) == 0 {
		return errors.New("coreDNS configmap not defined")
	}

	coreConfigMapName := coreDeployment.Spec.Template.Spec.Volumes[0].ConfigMap.Name

	coreConfigmap, err := client.CoreV1().ConfigMaps(coreDeployment.Namespace).Get(coreConfigMapName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if len(coreConfigmap.ObjectMeta.Labels) > 0 {
		if _, ok := coreConfigmap.ObjectMeta.Labels["traefik-mesh-patched"]; ok {
			return nil
		}
	}

	return errors.New("coreDNS not patched. Run ./i3o patch to update DNS")
}

func patchCoreDNS(client kubernetes.Interface, deploymentName, deploymentNamespace string) error {
	coreDeployment, err := client.AppsV1().Deployments(deploymentNamespace).Get(deploymentName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	log.Debugln("Patching CoreDNS configmap...")
	patched, err := patchCoreConfigMap(client, coreDeployment)
	if err != nil {
		return err
	}

	if !patched {
		log.Debugln("Restarting CoreDNS pods...")
		if err := restartCorePods(client, coreDeployment); err != nil {
			return err
		}
	}

	return nil
}

func patchCoreConfigMap(client kubernetes.Interface, coreDeployment *appsv1.Deployment) (bool, error) {
	var coreConfigMapName string
	if len(coreDeployment.Spec.Template.Spec.Volumes) == 0 {
		return false, errors.New("coreDNS configmap not defined")
	}

	coreConfigMapName = coreDeployment.Spec.Template.Spec.Volumes[0].ConfigMap.Name

	coreConfigMap, err := client.CoreV1().ConfigMaps(coreDeployment.Namespace).Get(coreConfigMapName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	if len(coreConfigMap.ObjectMeta.Labels) > 0 {
		if _, ok := coreConfigMap.ObjectMeta.Labels["traefik-mesh-patched"]; ok {
			log.Debugln("Configmap already patched...")
			return true, nil
		}
	}

	patchString := `loadbalance
    rewrite {
        name regex ([a-z]*)\.([a-z]*)\.traefik\.mesh traefik-{1}-{2}.traefik-mesh.svc.cluster.local
        answer name traefik-([a-z]*)-([a-z]*)\.traefik-mesh\.svc\.cluster\.local {1}.{2}.traefik.mesh
    }
`
	newCoreConfigmap := coreConfigMap
	oldData := newCoreConfigmap.Data["Corefile"]
	newData := strings.Replace(oldData, "loadbalance", patchString, 1)
	newCoreConfigmap.Data["Corefile"] = newData
	if len(newCoreConfigmap.ObjectMeta.Labels) == 0 {
		newCoreConfigmap.ObjectMeta.Labels = make(map[string]string)
	}
	newCoreConfigmap.ObjectMeta.Labels["traefik-mesh-patched"] = "true"

	if _, err = client.CoreV1().ConfigMaps(coreDeployment.Namespace).Update(newCoreConfigmap); err != nil {
		return false, err
	}

	return false, nil
}

func restartCorePods(client kubernetes.Interface, coreDeployment *appsv1.Deployment) error {
	coreLabelSelector := labels.Set(coreDeployment.Spec.Selector.MatchLabels).String()
	deploymentNamespace := coreDeployment.Namespace
	deploymentName := coreDeployment.Name

	corePods, err := client.CoreV1().Pods(deploymentNamespace).List(metav1.ListOptions{LabelSelector: coreLabelSelector})
	if err != nil {
		return err
	}

	for _, p := range corePods.Items {
		log.Infof("Deleting pod %s...\n", p.Name)
		if err := client.CoreV1().Pods(deploymentNamespace).Delete(p.Name, nil); err != nil {
			return err
		}
		time.Sleep(5 * time.Second)
		for {
			d, err := client.AppsV1().Deployments(deploymentNamespace).Get(deploymentName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if d.Status.ReadyReplicas == d.Status.Replicas {
				break
			}
			time.Sleep(5 * time.Second)
		}
	}
	return nil
}

// Contains tells whether a contains x.
func Contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}

// ServiceToMeshName converts a service with a namespace to a traefik-mesh ingressroute name
func ServiceToMeshName(serviceName string, namespace string) string {
	return fmt.Sprintf("traefik-%s-%s", namespace, serviceName)
}
