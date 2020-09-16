# Compatibility

Traefik Mesh supports, [similar to Kubernetes](https://kubernetes.io/docs/setup/release/version-skew-policy/#supported-versions), at least the latest three minor versions of Kubernetes, therefore currently:

* 1.17
* 1.18
* 1.19

General functionality cannot be guaranted for versions older than that. However, we expect it to work with Kubernetes down to 1.11 currently.

## Compatibility by Features

Some of Traefik Mesh's features are only supported on certain Kubernetes versions. 
Please see the table below.

| Features              | K8s 1.17 | K8s 1.18 | K8s 1.19 |
|-----------------------|----------|----------|----------|
| General functionality | ✔        | ✔        | ✔        |
| Service Topology      | ✔        | ✔        | ✔        |
