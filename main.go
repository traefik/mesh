/*
Copyright 2017 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Note: the example only works with the code within the same release/branch.
package main

import (
	"flag"
	"fmt"
	"path/filepath"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	podsClient := clientset.CoreV1().Pods(apiv1.NamespaceDefault)

	fmt.Printf("Listing pods in namespace %q:\n", apiv1.NamespaceDefault)
	podList, err := podsClient.List(metav1.ListOptions{})
	if err != nil {
		panic(err)
	}
	for _, p := range podList.Items {
		fmt.Printf(" * %s \n", p.Name)
	}

	serviceClient := clientset.CoreV1().Services(apiv1.NamespaceAll)

	fmt.Printf("Listing services in namespace %q:\n", apiv1.NamespaceAll)
	serviceList, err := serviceClient.List(metav1.ListOptions{})
	if err != nil {
		panic(err)
	}
	for _, s := range serviceList.Items {
		fmt.Printf(" * %s/%s \n", s.Namespace, s.Name)
	}

	fmt.Printf("Listing pods in namespace %q:\n", meshNamespace)
	podsClient = clientset.CoreV1().Pods(meshNamespace)
	podList, err = podsClient.List(metav1.ListOptions{})
	if err != nil {
		panic(err)
	}
	for _, p := range podList.Items {
		fmt.Printf(" * %s \n", p.Name)
	}

	fmt.Printf("Listing services in namespace %q:\n", meshNamespace)
	serviceClient = clientset.CoreV1().Services(meshNamespace)
	serviceList, err = serviceClient.List(metav1.ListOptions{})
	if err != nil {
		panic(err)
	}
	for _, s := range serviceList.Items {
		fmt.Printf(" * %s/%s \n", s.Namespace, s.Name)
	}

}
