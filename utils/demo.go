package utils

import (
	"github.com/containous/traefik/log"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func createDemoData(client kubernetes.Interface) error {
	deploymentList := &appsv1.DeploymentList{
		Items: []appsv1.Deployment{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "whoami",
					Namespace: "foo",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: Int32Ptr(2),
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
					Replicas: Int32Ptr(2),
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
					Replicas: Int32Ptr(1),
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
									Image: "giantswarm/tiny-tools:3.9",
									Command: []string{
										"sh",
										"-c",
										"sleep 1000",
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

	log.Debugln("Creating Demo Namespaces...")
	for _, n := range namespaceList.Items {
		_, err := client.CoreV1().Namespaces().Create(&n)
		if err != nil {
			log.Debugf("Namespace %s already exists...\n", n.Name)
		}
	}

	log.Debugln("Creating Demo Services...")
	for _, s := range serviceList.Items {
		_, err := client.CoreV1().Services(s.Namespace).Create(&s)
		if err != nil {
			log.Debugf("Service %s already exists...\n", s.Name)
		}
	}

	log.Debugln("Creating Demo Deployments...")
	for _, d := range deploymentList.Items {
		_, err := client.AppsV1().Deployments(d.Namespace).Create(&d)
		if err != nil {
			log.Debugf("Deployment %s already exists...\n", d.Name)
		}
	}

	return nil
}
