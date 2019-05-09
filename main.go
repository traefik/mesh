package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
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
	meshNamespace     string = "traefik-mesh"
	meshPodPrefix     string = "traefik"
	meshConfigmapName string = "traefik-mesh-config"
)

type traefikMeshConfig struct {
	Services []traefikMeshService
}

type traefikMeshService struct {
	ServicePort      int32
	ServiceName      string
	ServiceNamespace string
	Servers          []traefikMeshBackendServer
}

type traefikMeshBackendServer struct {
	Address string
	Port    int32
}

var demo bool
var kubeconfig string

func init() {
	flag.BoolVar(&demo, "demo", false, "install demo data")

	if home := homedir.HomeDir(); home != "" {
		flag.StringVar(&kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	}

	flag.Parse()

}

func main() {
	fmt.Println("Connecting to kubernetes...")
	clientset, err := buildClient()
	if err != nil {
		panic(err)
	}

	fmt.Println("Verifying mesh namespace exists...")
	if err := verifyNamespaceExists(clientset, meshNamespace); err != nil {
		panic(err)
	}

	fmt.Println("Creating demo data...")
	if demo {
		if err := createDemoData(clientset); err != nil {
			panic(err)
		}
	}

	fmt.Println("Listing services in all namespaces:")
	serviceListAll, err := clientset.CoreV1().Services(apiv1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		panic(err)
	}

	var serviceListNonMesh []apiv1.Service
	var meshServices []traefikMeshService

	for _, s := range serviceListAll.Items {
		if s.Namespace == meshNamespace {
			continue
		}

		serviceListNonMesh = append(serviceListNonMesh, s)

		fmt.Printf(" * %s/%s \n", s.Namespace, s.Name)

		if err := verifyMeshServiceExists(clientset, s.Namespace, s.Name); err != nil {
			panic(err)
		}

		var endpoints *apiv1.EndpointsList
		for {
			endpoints, err = clientset.CoreV1().Endpoints(s.Namespace).List(metav1.ListOptions{
				FieldSelector: fmt.Sprintf("metadata.name=%s", s.Name),
			})
			if err != nil {
				time.Sleep(time.Second * 5)
			} else if len(endpoints.Items[0].Subsets) == 0 {
				time.Sleep(time.Second * 5)
			} else {
				break
			}
		}

		// Verify that the expected amount of control nodes are listed in the endpoint list.

		var svr []traefikMeshBackendServer

		for _, e := range endpoints.Items[0].Subsets[0].Addresses {
			ip := e.IP
			port := endpoints.Items[0].Subsets[0].Ports[0].Port

			svr = append(svr, traefikMeshBackendServer{
				Address: ip,
				Port:    port,
			})
			fmt.Printf(" - Adding server %s:%d to routing config\n", ip, port)
		}

		meshService := traefikMeshService{
			ServiceName:      s.Name,
			ServiceNamespace: s.Namespace,
			ServicePort:      s.Spec.Ports[0].Port,
			Servers:          svr,
		}

		meshServices = append(meshServices, meshService)
	}

	meshConfig := traefikMeshConfig{
		Services: meshServices,
	}

	fmt.Printf("Generated Config: %+v\n", meshConfig)

	fmt.Println("Creating routing config for services...")
	if err := createRoutingConfig(clientset, meshConfig); err != nil {
		panic(err)
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

func buildClient() (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
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

func verifyMeshServiceExists(client *kubernetes.Clientset, name, namespace string) error {
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

func createRoutingConfig(client *kubernetes.Clientset, config traefikMeshConfig) error {
	t, _ := template.ParseFiles("templates/traefik-routing.tpl") // Parse template file.

	var tpl bytes.Buffer
	if err := t.Execute(&tpl, config); err != nil {
		return err
	}

	output := tpl.String()

	fmt.Printf("Templated config: %s", output)

	m, _ := client.CoreV1().ConfigMaps(meshNamespace).Get(meshConfigmapName, metav1.GetOptions{})
	if m != nil {
		// Config exists, update
	}

	return nil
}

func createDemoData(client *kubernetes.Clientset) error {
	deploymentList := &appsv1.DeploymentList{
		Items: []appsv1.Deployment{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "whoami",
					Namespace: "foo",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(2),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "whoami",
						},
					},
					Template: apiv1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "whoami",
							},
						},
						Spec: apiv1.PodSpec{
							Containers: []apiv1.Container{
								{
									Name:  "whoami",
									Image: "containous/whoami:v1.0.1",
									Ports: []apiv1.ContainerPort{
										{
											Name:          "http",
											Protocol:      apiv1.ProtocolTCP,
											ContainerPort: 80,
										},
									},
								},
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "whoami",
					Namespace: "bar",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(2),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "whoami",
						},
					},
					Template: apiv1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "whoami",
							},
						},
						Spec: apiv1.PodSpec{
							Containers: []apiv1.Container{
								{
									Name:  "whoami",
									Image: "containous/whoami:v1.0.1",
									Ports: []apiv1.ContainerPort{
										{
											Name:          "http",
											Protocol:      apiv1.ProtocolTCP,
											ContainerPort: 80,
										},
									},
								},
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "demo-shell",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "demo",
						},
					},
					Template: apiv1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "demo",
							},
						},
						Spec: apiv1.PodSpec{
							Containers: []apiv1.Container{
								{
									Name:  "demo",
									Image: "traefik:alpine",
									Ports: []apiv1.ContainerPort{
										{
											Name:          "http",
											Protocol:      apiv1.ProtocolTCP,
											ContainerPort: 80,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	namespaceList := &apiv1.NamespaceList{
		Items: []apiv1.Namespace{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: apiv1.NamespaceSpec{},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bar",
				},
				Spec: apiv1.NamespaceSpec{},
			},
		},
	}

	serviceList := &apiv1.ServiceList{
		Items: []apiv1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "zip",
					Namespace: "foo",
				},
				Spec: apiv1.ServiceSpec{
					Ports: []apiv1.ServicePort{
						{
							Port: 80,
						},
					},
					Selector: map[string]string{
						"app": "whoami",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dee",
					Namespace: "foo",
				},
				Spec: apiv1.ServiceSpec{
					Ports: []apiv1.ServicePort{
						{
							Port: 80,
						},
					},
					Selector: map[string]string{
						"app": "whoami",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "doo",
					Namespace: "bar",
				},
				Spec: apiv1.ServiceSpec{
					Ports: []apiv1.ServicePort{
						{
							Port: 80,
						},
					},
					Selector: map[string]string{
						"app": "whoami",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dah",
					Namespace: "bar",
				},
				Spec: apiv1.ServiceSpec{
					Ports: []apiv1.ServicePort{
						{
							Port: 80,
						},
					},
					Selector: map[string]string{
						"app": "whoami",
					},
				},
			},
		},
	}

	fmt.Println("Creating Demo Namespaces...")
	for _, n := range namespaceList.Items {
		_, err := client.CoreV1().Namespaces().Create(&n)
		if err != nil {
			fmt.Printf("Namespace %s already exists...\n", n.Name)
		}
	}

	fmt.Println("Creating Demo Services...")
	for _, s := range serviceList.Items {
		_, err := client.CoreV1().Services(s.Namespace).Create(&s)
		if err != nil {
			fmt.Printf("Service %s already exists...\n", s.Name)
		}
	}

	fmt.Println("Creating Demo Deployments...")
	for _, d := range deploymentList.Items {
		_, err := client.AppsV1().Deployments(d.Namespace).Create(&d)
		if err != nil {
			fmt.Printf("Deployment %s already exists...\n", d.Name)
		}
	}

	return nil
}

func int32Ptr(i int32) *int32 { return &i }
