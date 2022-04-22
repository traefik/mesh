---
title: "Traefik Mesh Installation Documentation"
description: "There are different ways you can install Traefik Mesh, a simple and lightweight service mesh, in your cluster. Read the technical documentation."
---

# Installation

To install Traefik Mesh, the installation method is quite simple:

```bash
helm repo add traefik-mesh https://helm.traefik.io/mesh
helm repo update
```

Install Traefik Mesh Helm Chart:

```bash
helm install traefik-mesh traefik-mesh/traefik-mesh
```

## Install from source

!!! Note "Supported Installations"
    Please be aware that the supported installation method is via Helm, using official releases.
    If you want to build/install/run Traefik Mesh from source, we may not be able to provide support.
    Installing from source is intended for development/contributing.

To build the image locally, run:

```shell
make
```

You will then be able to use the tagged image as your image in your `values.yaml` file.

### Deploy Helm Chart

To deploy the Helm Chart, run:

```shell
helm install traefik-mesh traefik-mesh/traefik-mesh --set controller.image.pullPolicy=IfNotPresent --set controller.image.tag=latest
```

## Access Control List

By default, Traefik Mesh does not restrict traffic between pods and services. However, some scenarios require more control over the rules for internal communication.
The Access Control List mode (ACL) requires a set of rules to explicitly allow traffic between different resources.

To enable ACL, install Traefik Mesh in ACL mode by setting the `acl` Helm Chart option to `true`.

```bash
helm install traefik-mesh --namespace=traefik-mesh traefik-mesh/traefik-mesh --set acl=true
```

Traefik Mesh supports the [SMI specification](https://smi-spec.io/) which defines a set of custom resources
to provide a fine-grained control over instrumentation, routing and access control of east-west communications.

!!! Note "CRDs"
    Helm v3 will install automatically the CRDs in the `/crds` directory.
    If you are (re)installing into a cluster with the CRDs already present, Helm may print a warning.
    If you do not want to install them, or want to avoid the warning, use the new `--skip-crds` flag.
    More information can be found in the [Helm documentation](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#method-1-let-helm-do-it-for-you).

## Platform recommendations

Traefik Mesh works on Kubernetes environments that conforms to the global Kubernetes specification.
That being said, we have had users encounter issues when using variants such as minikube, microk8s,
and other development distributions.

Traefik Mesh runs without issue on most public clouds (AWS, GKE, Azure, DigitalOcean, and more).
If you want to run Traefik Mesh in development, we would recommend using [k3s](https://k3s.io/), as it is fully conformant.
We use k3s in Traefik Mesh's integration tests, so you can be sure that it works properly.

If you encounter issues on variants such as minikube or microk8s, please try and reproduce the issue on k3s.
If you are unable to reproduce, it may be an issue with the distribution behaving differently than official Kubernetes.

## Verify your installation

You can check that Traefik Mesh has been installed properly by running the following command:

```bash tab="Command"
kubectl get all -n traefik-mesh
```

```text tab="Expected Output"

NAME                                            READY   STATUS    RESTARTS   AGE
pod/traefik-mesh-controller-676fb86b89-pj8ph    1/1     Running   0          11s
pod/traefik-mesh-proxy-w62z5                    1/1     Running   0          11s
pod/traefik-mesh-proxy-zjlpf                    1/1     Running   0          11s

NAME                                DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
daemonset.apps/traefik-mesh-proxy   2         2         0       2            0           <none>          29s

NAME                                      DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/traefik-mesh-controller   1         1         1            0           28s

NAME                                                 DESIRED   CURRENT   READY   AGE
replicaset.apps/traefik-mesh-controller-676fb86b89   1         1         0       28s
```

## Usage

To use Traefik Mesh, instead of referencing services via their normal `<servicename>.<namespace>`, instead use `<servicename>.<namespace>.traefik.mesh`.
This will access the Traefik Mesh service mesh, and will allow you to route requests through Traefik Mesh.

By default, Traefik Mesh is opt-in, meaning you have to use the Traefik Mesh service names to access the mesh, so you can have some services running through the mesh, and some services not.
