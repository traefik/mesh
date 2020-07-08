# Maesh

Maesh is a simple, yet full-featured service mesh. It is container-native and fits as your de-facto service mesh in your 
Kubernetes cluster. It supports the latest Service Mesh Interface specification [SMI](https://smi-spec.io/) that facilitates 
integration with pre-existing solution.

Moreover, Maesh is opt-in by default, which means that your existing services are 
unaffected until you decide to add them to the mesh.

## Prerequisites

- Kubernetes 1.11+
- CoreDNS/KubeDNS installed as [Cluster DNS Provider](https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/) (versions 1.3+ supported)
- [Helm v3](https://helm.sh/docs/intro/install/)

## Installing the Chart

To install the chart with the release name `maesh`:

```bash
$ helm repo add maesh https://containous.github.io/maesh/charts
$ helm repo update
$ helm install maesh maesh/maesh
```

You can use the `--namespace my-namespace` flag to deploy Maesh in a custom namespace and the `--set "key1=val1,key2=val2,..."`
flag to configure it. Where `key1=val1`, `key2=val2`, `...` are chart values that you can find at 
https://github.com/containous/maesh/blob/master/helm/chart/maesh/values.yaml.

## Uninstalling the Chart

To uninstall the chart with the release name `maesh`:

```bash
$ helm uninstall maesh
```

## Contributing

If you want to contribute to this chart, please read the [Guidelines](./Guidelines.md).
