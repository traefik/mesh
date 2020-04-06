<p align="center">
<img width="400" src="docs/content/assets/img/maesh.png" alt="Maesh" title="Maesh" />
</p>


[![Build Status](https://containous.semaphoreci.com/badges/maesh/branches/master.svg?style=shields)](https://containous.semaphoreci.com/projects/maesh)
[![Docs](https://img.shields.io/badge/docs-current-brightgreen.svg)](https://docs.mae.sh)
[![GitHub](https://img.shields.io/github/license/containous/maesh)](https://github.com/containous/maesh/blob/master/LICENSE)
[![release](https://img.shields.io/github/tag-date/containous/maesh.svg)](https://github.com/containous/maesh/releases)
[![Build Status](https://travis-ci.com/containous/maesh.svg?branch=master)](https://travis-ci.com/containous/maesh)
[![Discourse status](https://img.shields.io/discourse/https/community.containo.us/status?label=Community&style=social)](https://community.containo.us/c/maesh)

## Maesh: Simpler Service Mesh

Maesh is a simple, yet full-featured service mesh. 
It is container-native and fits as your de-facto service mesh in your Kubernetes cluster. 
It supports the latest Service Mesh Interface specification [SMI](https://smi-spec.io) that facilitates integration with pre-existing solution. 
Moreover, Maesh is opt-in by default, 
which means that your existing services are unaffected until you decide to add them to the mesh.

<p align="center">
<a href="https://smi-spec.io" target="_blank"><img width="150" src="docs/content/assets/img/smi.png" alt="SMI" title="SMI" /></a>
</p>


## Non-Invasive Service Mesh

Maesh does not use any sidecar container but handles routing through proxy endpoints running on each node. 
The mesh controller runs in a dedicated pod and handles all the configuration parsing and deployment to the proxy nodes. 
Maesh supports multiple configuration options: annotations on user service objects, and SMI objects. 
Not using sidecars means that Maesh does not modify your kubernetes objects, and does not modify your traffic without your knowledge. 
Using the Maesh endpoints is all that is required.

<p align="center">
<img width="400" src="docs/content/assets/img/before-maesh-graphic.png" alt="Maesh" title="Maesh" />
<img width="400" src="docs/content/assets/img/after-maesh-graphic.png" alt="Maesh" title="Maesh" />
</p>

## Prerequisites

To run this app, you require the following:

- Kubernetes 1.11+
- CoreDNS installed as [Cluster DNS Provider](https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/) (versions 1.3+ supported)
- Helm v3

## Install (Helm v3 only)

```shell
helm repo add maesh https://containous.github.io/maesh/charts
helm repo update
helm install maesh maesh/maesh
```

You can find the complete documentation at https://docs.mae.sh.


## Contributing

[Contributing guide](CONTRIBUTING.md).
