# Maesh

The simpler service mesh controller

[![Build Status](https://semaphoreci.com/api/v1/projects/d10436a4-dcfb-454a-b19e-a9c96370b92d/2743246/badge.svg)](https://semaphoreci.com/containous/maesh)

## Prerequisites

To run this app, you require the following:

- Kubernetes 1.11+
- CoreDNS installed as Cluster DNS Provider (https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/)
- Helm v2 with working tiller service account (may require updating tiller or creating the service account/`helm init`)

## Installation

To install maesh, the installation method is quite simple:

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

`helm install helm/chart/maesh --namespace maesh --set controller.image.pullPolicy=IfNotPresent --set controller.image.tag=latest`

Note: The chart uses the `local-path` provisioner for k3s, but you can override that using:

`helm install helm/chart/maesh --namespace maesh --set controller.image.pullPolicy=IfNotPresent --set controller.image.tag=latest --set metrics.storageClass=hostpath`

## Usage

To use maesh, instead of referencing services via their normal `<servicename>.<namespace>`, instead use `<servicename>.<namespace>.maesh`.
This will access the maesh service mesh, and will allow you to route requests through maesh.

By default, maesh is opt-in, meaning you have to use the maesh service names to access the mesh, so you can have some services running through the mesh, and some services not.
