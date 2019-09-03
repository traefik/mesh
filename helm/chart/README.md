# Maesh Helm Chart

## Prerequisites

With the command `helm version`, make sure that you have:
- Helm [installed](https://helm.sh/docs/using_helm/#installing-helm) 
- Tiller [deployed](https://helm.sh/docs/using_helm/#installing-tiller) to your cluster 

Add Maesh's chart repository to Helm:

```bash
helm repo add maesh https://containous.github.io/maesh/charts
```

You can update the chart repository by running:

```bash
helm repo update
```

## Deploy Maesh

### Deploy Maesh with Default Config

```bash
helm install --name=maesh maesh/maesh
```

### Deploy Maesh in a Custom Namespace

```bash
helm install --name=maesh --namespace=maesh maesh/maesh
```

### Deploy with Custom Configuration

```bash
helm install --name=maesh --namespace=maesh --set "key1=val1,key2=val2,..." maesh/maesh
```

Where `key1=val1`, `key2=val2`, `...` are chart values that you can find at <https://github.com/containous/maesh/blob/master/helm/chart/maesh/values.yaml>.
