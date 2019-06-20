# i3o

The simple service mesh controller

## Prerequisites

To run this app, you require the following:

- Kubernetes 1.11+
- CoreDNS installed as Cluster DNS Provider (https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/)
- Helm v2 with working tiller service account (may require updating tiller or creating the service account/`helm init`)

## Installation

To install i3o, the installation method is quite simple:

### Pre-release installation

To run the installer pre-release, you can do one of the following:

- Log into dockerhub with your credentials on the kube machine to allow the private image pulls
- Create imagepullsecrets for the image
- Build the image locally and modify the imagepullpolicy to not pull if exists

The third option is the one we recommend here.

### Pre-release image build

To build the image locally:

run `make` to build the binary and build/tag the local image.

### Deploy helm chart

To deploy the helm chart, run:

`helm install helm/chart/i3o --set image.pullPolicy=IfNotPresent --set image.tag=latest`

## Usage

To use i3o, instead of referencing services via their normal `<servicename>.<namespace>`, instead use `<servicename>.<namespace>.traefik.mesh`.
This will access the i3o service mesh, and will allow you to route requests through i3o.

By default, i3o is opt-in, meaning you have to use the i3o service names to access the mesh, so you can have some services running through the mesh, and some services not.
