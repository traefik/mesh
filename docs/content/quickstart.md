# Quickstart

Maesh can be installed in your cluster without affecting any running services.
It will safely install itself via the helm chart, and will be ready for use immediately after.

It can be installed by running:

```shell
helm repo add maesh https://containous.github.io/maesh/charts
helm repo update
helm install --name=maesh --namespace=maesh maesh/maesh
```
