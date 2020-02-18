# Installation

To install maesh, the installation method is quite simple:

```bash
helm repo add maesh https://containous.github.io/maesh/charts
helm repo update
```

Install maesh helm chart:

```bash
helm install maesh maesh/maesh
```

## Install from source

To build the image locally, run:

```shell
make
```

 to build the binary and build/tag the local image.

## Deploy helm chart

??? Note "Helm V3"
    Please keep in mind, that our current Helm Chart (v1.0.0) is v2 compatible only. The v3 compatible Chart will be released with v1.1 of Maesh.

To deploy the helm chart, run:

```shell
helm install maesh helm/chart/maesh --set controller.image.pullPolicy=IfNotPresent --set controller.image.tag=latest
```

## KubeDNS support

Maesh can support KubeDNS

```bash
helm install maesh maesh/maesh --set kubedns=true
```

With this parameter Maesh will install a CoreDNS as a daemonset.
KubeDNS will be patched with [stubDomains](https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/#example-stub-domain)

## Custom cluster domain

If you use a cluster domain other than `cluster.local` set it by using the `clusterDomain` parameter:

```bash
helm install maesh maesh/maesh --set clusterDomain=my.custom.domain.com
```

## Service Mesh Interface

Maesh supports the [SMI specification](https://smi-spec.io/) which defines a set of custom resources
to provide a fine-grained control over instrumentation, routing and access control of east-west communications.

To enable SMI, install maesh in SMI mode by setting the `smi.enable` helm chart option to true.

```bash
helm install maesh --namespace=maesh maesh/maesh --set smi.enable=true`
```

- The `smi.enable` option makes Maesh process SMI resources.

!!! Note CRDs
    Helm v3 automatically will install the CRDs in the `/crds` directory.
    If you are re-installing into a cluster with the CRDs already present, helm may give you a warning.
    If you do not want to install them, or want to avoid the warning during a re-install,
    please use the new `--skip-crds` flag.
    More informationcan be found on the [helm documentation](https://helm.sh/docs/topics/chart_best_practices/custom_resource_definitions/#method-1-let-helm-do-it-for-you)

## Platform recommendations

Maesh will work on pretty much any kubernetes environment that conforms to the global kubernetes specification.
That being said, we have had users encounter issues when using variants such as minikube, microk8s,
and other development distributions.

Maesh runs without issue on most public clouds (AWS, GKE, Azure, DigitalOcean, and more).
If you want to run Maesh in development, we would recommend using [k3s](https://k3s.io/), as it is fully conformant.
We use k3s in Maesh's integration tests, so you can be sure that it works properly.

If you encounter issues on variants such as minikube or microk8s, please try and reproduce the issue on k3s.
If you are unable to reproduce, it may be an issue with the distribution behaving differently than official kubernetes.

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
