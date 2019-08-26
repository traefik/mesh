# Maesh Helm Chart

## Prerequisites

Make sure you have:
- Helm [installed](https://helm.sh/docs/using_helm/#installing-helm) 
- Tiller [deployed](https://helm.sh/docs/using_helm/#installing-tiller) to your cluster 

Add Maesh's chart repository to Helm:

```bash
$ helm repo add maesh https://containous.github.io/maesh/charts
```

You can update the chart repository by running:

```bash
$ helm repo update
```

## Deploy Maesh

### Deploy Maesh with default config

```bash
$ helm install maesh
```

### Deploy Maesh in a custom namespace

```bash
$ helm install maesh --namespace=maesh-custom maesh
```

### Deploy with custom config

```bash
$ helm install maesh --set "key1=val1,key2=val2,..."
```
