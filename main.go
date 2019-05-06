package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const (
	meshNamespace string = "traefik-mesh"
	meshPodPrefix string = "traefik"
)

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	fmt.Println("Verifying mesh namespace exists...")
	if err := verifyNamespaceExists(clientset, meshNamespace); err != nil {
		panic(err)
	}

	fmt.Println("Listing services in all namespaces:")
	serviceListAll, err := clientset.CoreV1().Services(apiv1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		panic(err)
	}

	for _, s := range serviceListAll.Items {
		if s.Namespace == meshNamespace {
			continue
		}

		fmt.Printf(" * %s/%s \n", s.Namespace, s.Name)

		if err := verifyServiceExists(clientset, s.Namespace, s.Name); err != nil {
			panic(err)
		}
	}

	fmt.Printf("Listing services in mesh namespace %q:\n", meshNamespace)
	serviceListMesh, err := clientset.CoreV1().Services(meshNamespace).List(metav1.ListOptions{})
	if err != nil {
		panic(err)
	}
	for _, s := range serviceListMesh.Items {
		fmt.Printf(" * %s/%s \n", s.Namespace, s.Name)
	}

	fmt.Println("Patching CoreDNS...")
	if err := patchCoreDNS(clientset, "coredns", "kube-system"); err != nil {
		panic(err)
	}
}

func verifyNamespaceExists(client *kubernetes.Clientset, namespace string) error {
	_, err := client.CoreV1().Namespaces().Get(meshNamespace, metav1.GetOptions{})
	if err != nil {
		ns := &apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: meshNamespace,
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

func verifyServiceExists(client *kubernetes.Clientset, name, namespace string) error {
	meshServiceName := fmt.Sprintf("%s-%s-%s", meshPodPrefix, namespace, name)
	meshServiceInstance, err := client.CoreV1().Services(meshNamespace).Get(meshServiceName, metav1.GetOptions{})
	if meshServiceInstance == nil || err != nil {
		svc := &apiv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      meshServiceName,
				Namespace: meshNamespace,
			},
			Spec: apiv1.ServiceSpec{
				Ports: []apiv1.ServicePort{
					{
						Port: 80,
					},
				},
				Selector: map[string]string{
					"mesh": "traefik-mesh",
				},
			},
		}

		_, err := client.CoreV1().Services(meshNamespace).Create(svc)
		if err != nil {
			return err
		}
	}
	return nil
}

func patchCoreDNS(client *kubernetes.Clientset, deploymentName, deploymentNamespace string) error {
	coreDeployment, err := client.AppsV1().Deployments(deploymentNamespace).Get(deploymentName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	fmt.Println("Patching CoreDNS configmap...")
	patched, err := patchCoreConfigmap(client, coreDeployment)
	if err != nil {
		return err
	}

	if !patched {
		fmt.Println("Restarting CoreDNS pods...")
		if err := restartCorePods(client, coreDeployment); err != nil {
			return err
		}
	}

	return nil
}

func restartCorePods(client *kubernetes.Clientset, coreDeployment *appsv1.Deployment) error {
	coreLabelSelector := labels.Set(coreDeployment.Spec.Selector.MatchLabels).String()
	deploymentNamespace := coreDeployment.Namespace
	deploymentName := coreDeployment.Name

	corePods, err := client.CoreV1().Pods(deploymentNamespace).List(metav1.ListOptions{LabelSelector: coreLabelSelector})
	if err != nil {
		return err
	}
	for _, p := range corePods.Items {
		fmt.Printf("Deleting pod %s...\n", p.Name)
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

func patchCoreConfigmap(client *kubernetes.Clientset, coreDeployment *appsv1.Deployment) (bool, error) {
	coreConfigmapName := coreDeployment.Spec.Template.Spec.Volumes[0].ConfigMap.Name
	//JESUS

	coreConfigmap, err := client.CoreV1().ConfigMaps(coreDeployment.Namespace).Get(coreConfigmapName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	if len(coreConfigmap.ObjectMeta.Labels) > 0 {
		if _, ok := coreConfigmap.ObjectMeta.Labels["traefik-mesh-patched"]; ok {
			fmt.Println("Configmap already patched...")
			return true, nil
		}
	}

	patchString := `loadbalance
    rewrite {
        name regex ([a-z]*)\.([a-z]*)\.traefik\.mesh traefik-{1}-{2}.traefik-mesh.svc.cluster.local
        answer name traefik-([a-z]*)-([a-z]*)\.traefik-mesh\.svc\.cluster\.local {1}.{2}.traefik.mesh
    }
`
	newCoreConfigmap := coreConfigmap
	oldData := newCoreConfigmap.Data["Corefile"]
	newData := strings.Replace(oldData, "loadbalance", patchString, 1)
	newCoreConfigmap.Data["Corefile"] = newData
	if len(newCoreConfigmap.ObjectMeta.Labels) == 0 {
		newCoreConfigmap.ObjectMeta.Labels = make(map[string]string)
	}
	newCoreConfigmap.ObjectMeta.Labels["traefik-mesh-patched"] = "true"

	_, err = client.CoreV1().ConfigMaps(coreDeployment.Namespace).Update(newCoreConfigmap)
	if err != nil {
		return false, err
	}

	return false, nil
}
