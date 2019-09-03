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

## Installation namespace

Maesh does not _need_ to be installed into the maesh namespace, 
but it does need to be installed into its _own_ namespace, separate from user namespaces.

## Usage

To use maesh, instead of referencing services via their normal `<servicename>.<namespace>`, instead use `<servicename>.<namespace>.maesh`.
This will access the maesh service mesh, and will allow you to route requests through maesh.

By default, maesh is opt-in, meaning you have to use the maesh service names to access the mesh, so you can have some services running through the mesh, and some services not.
