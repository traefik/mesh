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

## SMI Specification support

Traefik Mesh is based on the latest version of the SMI specification:

| API Group          | API Version                                                                                                             |
|--------------------|-------------------------------------------------------------------------------------------------------------------------|
| access.smi-spec.io | [v1alpha2](https://github.com/servicemeshinterface/smi-spec/blob/master/apis/traffic-access/v1alpha2/traffic-access.md) |
| specs.smi-spec.io  | [v1alpha3](https://github.com/servicemeshinterface/smi-spec/blob/master/apis/traffic-specs/v1alpha3/traffic-specs.md)   |
| split.smi-spec.io  | [v1alpha3](https://github.com/servicemeshinterface/smi-spec/blob/master/apis/traffic-split/v1alpha3/traffic-split.md)   |
