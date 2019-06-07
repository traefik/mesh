package utils

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

const (
	MeshNamespace     string = "traefik-mesh"
	MeshPodPrefix     string = "traefik"
	MeshConfigmapName string = "traefik-mesh-config"
	MeshConfigmapKey  string = "traefik.toml"
)

type TraefikMeshConfig struct {
	Services []TraefikMeshService
}

type TraefikMeshService struct {
	ServicePort      int32
	ServiceName      string
	ServiceNamespace string
	Servers          []TraefikMeshBackendServer
}

type TraefikMeshBackendServer struct {
	Address string
	Port    int32
}

// InitCluster is used to initialize a kubernetes cluster with a variety of configuration options
func InitCluster(client kubernetes.Interface, demoData bool) error {
	log.Infoln("Preparing Cluster...")
	defer log.Infoln("Cluster Preparation Complete...")

	log.Debugln("Verifying mesh namespace exists...")
	if err := verifyNamespaceExists(client, MeshNamespace); err != nil {
		return err
	}

	if demoData {
		log.Debugln("Creating demo data...")
		if err := createDemoData(client); err != nil {
			return err
		}
	}

	log.Debugln("Patching CoreDNS...")
	if err := patchCoreDNS(client, "coredns", metav1.NamespaceSystem); err != nil {
		return err
	}

	log.Debugln("Creating Traefik Mesh Daemonset...")
	if err := createTraefikMeshDaemonset(client, MeshNamespace); err != nil {
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

func patchCoreDNS(client kubernetes.Interface, deploymentName, deploymentNamespace string) error {
	coreDeployment, err := client.AppsV1().Deployments(deploymentNamespace).Get(deploymentName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	log.Debugln("Patching CoreDNS configmap...")
	patched, err := patchCoreConfigmap(client, coreDeployment)
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

func patchCoreConfigmap(client kubernetes.Interface, coreDeployment *appsv1.Deployment) (bool, error) {
	coreConfigmapName := coreDeployment.Spec.Template.Spec.Volumes[0].ConfigMap.Name
	//JESUS

	coreConfigmap, err := client.CoreV1().ConfigMaps(coreDeployment.Namespace).Get(coreConfigmapName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	if len(coreConfigmap.ObjectMeta.Labels) > 0 {
		if _, ok := coreConfigmap.ObjectMeta.Labels["traefik-mesh-patched"]; ok {
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

func createTraefikMeshDaemonset(client kubernetes.Interface, meshNamespace string) error {
	traefikDaemonset := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "traefik-mesh-node",
			Namespace: meshNamespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "traefik-mesh-node",
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "traefik-mesh-node",
					},
				},
				Spec: apiv1.PodSpec{
					Containers: []apiv1.Container{
						{
							Name:  "traefik",
							Image: "traefik:v2.0.0-alpha4-alpine",
							Ports: []apiv1.ContainerPort{
								{
									Name:          "http",
									Protocol:      apiv1.ProtocolTCP,
									ContainerPort: 8000,
								},
							},
							VolumeMounts: []apiv1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/traefik",
								},
							},
						},
					},
					Volumes: []apiv1.Volume{
						{
							Name: "config",
							VolumeSource: apiv1.VolumeSource{
								ConfigMap: &apiv1.ConfigMapVolumeSource{
									LocalObjectReference: apiv1.LocalObjectReference{
										Name: MeshConfigmapName,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := client.AppsV1().DaemonSets(meshNamespace).Create(traefikDaemonset)
	if err != nil {
		log.Debugf("Daemonset %s already exists...\n", traefikDaemonset.Name)
	}

	return nil
}

// CreateMeshConfig parses the kubernetes service list, and creates a structure for building configurations from.
func CreateMeshConfig(client kubernetes.Interface) (meshConfig *TraefikMeshConfig, err error) {
	log.Infoln("Creating mesh structures for config...")
	defer log.Infoln("Config Structure Creation Complete...")

	log.Debugln("Listing services in all namespaces:")
	serviceListAll, err := client.CoreV1().Services(apiv1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var serviceListNonMesh []apiv1.Service
	var meshServices []TraefikMeshService

	for _, s := range serviceListAll.Items {
		if s.Namespace == MeshNamespace {
			continue
		}

		serviceListNonMesh = append(serviceListNonMesh, s)

		log.Debugf(" * %s/%s \n", s.Namespace, s.Name)

		if err := verifyMeshServiceExists(client, s.Namespace, s.Name); err != nil {
			panic(err)
		}

		var endpoints *apiv1.EndpointsList
		for {
			endpoints, err = client.CoreV1().Endpoints(s.Namespace).List(metav1.ListOptions{
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

		var svr []TraefikMeshBackendServer

		for _, e := range endpoints.Items[0].Subsets[0].Addresses {
			ip := e.IP
			port := endpoints.Items[0].Subsets[0].Ports[0].Port

			svr = append(svr, TraefikMeshBackendServer{
				Address: ip,
				Port:    port,
			})
			log.Debugf(" - Adding server %s:%d to routing config\n", ip, port)
		}

		meshService := TraefikMeshService{
			ServiceName:      s.Name,
			ServiceNamespace: s.Namespace,
			ServicePort:      s.Spec.Ports[0].Port,
			Servers:          svr,
		}

		meshServices = append(meshServices, meshService)
	}
	return &TraefikMeshConfig{
		Services: meshServices,
	}, nil

}

func verifyMeshServiceExists(client kubernetes.Interface, name, namespace string) error {
	meshServiceName := fmt.Sprintf("%s-%s-%s", MeshPodPrefix, namespace, name)
	meshServiceInstance, err := client.CoreV1().Services(MeshNamespace).Get(meshServiceName, metav1.GetOptions{})
	if meshServiceInstance == nil || err != nil {
		svc := &apiv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      meshServiceName,
				Namespace: MeshNamespace,
			},
			Spec: apiv1.ServiceSpec{
				Ports: []apiv1.ServicePort{
					{
						Name:       "web",
						Port:       80,
						TargetPort: intstr.FromInt(8000),
					},
				},
				Selector: map[string]string{
					"app": "traefik-mesh-node",
				},
			},
		}

		_, err := client.CoreV1().Services(MeshNamespace).Create(svc)
		if err != nil {
			return err
		}
	}
	return nil
}

// CreateRoutingConfigmap takes a config of traefik mesh, and creates the associated configmap
func CreateRoutingConfigmap(client kubernetes.Interface, config *TraefikMeshConfig) error {
	log.Infoln("Creating routing configmap...")
	defer log.Infoln("Configmap Creation Complete...")

	t, _ := template.ParseFiles("templates/traefik-routing.tpl") // Parse template file.

	var tpl bytes.Buffer
	if err := t.Execute(&tpl, &config); err != nil {
		return err
	}

	output := tpl.String()

	meshConfigmapList, _ := client.CoreV1().ConfigMaps(MeshNamespace).List(metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", MeshConfigmapName),
	})
	if len(meshConfigmapList.Items) > 0 {
		// Config exists, update
		log.Debugln("Updating configmap...")

		m, _ := client.CoreV1().ConfigMaps(MeshNamespace).Get(MeshConfigmapName, metav1.GetOptions{})
		newConfigmap := m
		newConfigmap.Data[MeshConfigmapKey] = output
		_, err := client.CoreV1().ConfigMaps(MeshNamespace).Update(newConfigmap)
		if err != nil {
			return err
		}
		return nil
	}

	log.Debugln("Creating new configmap...")

	newConfigmap := &apiv1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MeshConfigmapName,
			Namespace: MeshNamespace,
		},
		Data: map[string]string{
			MeshConfigmapKey: output,
		},
	}
	_, err := client.CoreV1().ConfigMaps(MeshNamespace).Create(newConfigmap)
	if err != nil {
		return err
	}
	return nil
}

// Int32Ptr converts an int32 to a pointer.
func Int32Ptr(i int32) *int32 { return &i }

// Contains tells whether a contains x.
func Contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}
