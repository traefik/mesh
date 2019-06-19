# i3o

The simple service mesh controller

## Prerequisites

To run this app, you require the following:

- Kubernetes 1.11+ (Not sure, but probably close to the oldest version supported)
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



