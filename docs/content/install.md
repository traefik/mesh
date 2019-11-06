# Installation

To install maesh, the installation method is quite simple:

```bash
helm repo add maesh https://containous.github.io/maesh/charts
helm repo update
```

Install maesh helm chart:

```bash
helm install --name=maesh --namespace=maesh maesh/maesh
```

## Install from source

To build the image locally, run:

```shell
make
```

 to build the binary and build/tag the local image.

## Deploy helm chart

To deploy the helm chart, run:

```shell
helm install helm/chart/maesh --namespace maesh --set controller.image.pullPolicy=IfNotPresent --set controller.image.tag=latest
```

## KubeDNS support

Maesh can support KubeDNS

```bash
helm install --name=maesh --namespace=maesh maesh/maesh --set kubedns=true
```

With this parameter Maesh will install a CoreDNS as a daemonset.
KubeDNS will be patched with [stubDomains](https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/#example-stub-domain)

## Custom cluster domain

If you use a cluster domain other than `cluster.local` set it by using the `clusterDomain` parameter:

```bash

helm install --name=maesh --namespace=maesh maesh/maesh --set clusterDomain=my.custom.domain.com
```

## Installation namespace

Maesh does not _need_ to be installed into the `maesh` namespace, 
but it does need to be installed into its _own_ namespace, separate from user namespaces.

## Verify your installation

You can check that Maesh has been installed properly by running the following command:

```bash tab="Command"
kubectl get all -n maesh
```

```text tab="Expected Output"

NAME                                    READY   STATUS    RESTARTS   AGE
pod/maesh-controller-676fb86b89-pj8ph   1/1     Running   0          11s
pod/maesh-mesh-w62z5                    1/1     Running   0          11s
pod/maesh-mesh-zjlpf                    1/1     Running   0          11s

NAME                     TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
service/maesh-mesh-api   ClusterIP   100.69.177.254   <none>        8080/TCP   29s

NAME                        DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
daemonset.apps/maesh-mesh   2         2         0       2            0           <none>          29s

NAME                               DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/maesh-controller   1         1         1            0           28s

NAME                                          DESIRED   CURRENT   READY   AGE
replicaset.apps/maesh-controller-676fb86b89   1         1         0       28s
```

## Usage

To use maesh, instead of referencing services via their normal `<servicename>.<namespace>`, instead use `<servicename>.<namespace>.maesh`.
This will access the maesh service mesh, and will allow you to route requests through maesh.

By default, maesh is opt-in, meaning you have to use the maesh service names to access the mesh, so you can have some services running through the mesh, and some services not.
