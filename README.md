<p align="center">
<img width="400" src="docs/content/assets/img/traefik-mesh.png" alt="Traefik Mesh" title="Traefik Mesh" />
</p>


[![Travis CI Build Status](https://travis-ci.com/traefik/mesh.svg?branch=master)](https://travis-ci.com/traefik/mesh)
[![Semaphore CI Build Status](https://traefik.semaphoreci.com/badges/mesh/branches/master.svg?style=shields)](https://traefik.semaphoreci.com/projects/mesh)
[![Docs](https://img.shields.io/badge/docs-current-brightgreen.svg)](https://doc.traefik.io/mesh)
[![Go Report Card](https://goreportcard.com/badge/github.com/traefik/mesh)](https://goreportcard.com/report/github.com/traefik/mesh)
[![Release](https://img.shields.io/github/tag-date/traefik/mesh.svg)](https://github.com/traefik/mesh/releases)
[![GitHub](https://img.shields.io/github/license/traefik/mesh)](https://github.com/traefik/mesh/blob/master/LICENSE)
[![Discourse status](https://img.shields.io/discourse/https/community.traefik.io/status?label=Community&style=social)](https://community.traefik.io/c/traefik-mesh)

## Traefik Mesh: Simpler Service Mesh

Traefik Mesh is a simple, yet full-featured service mesh. 
It is container-native and fits as your de-facto service mesh in your Kubernetes cluster. 
It supports the latest Service Mesh Interface specification [SMI](https://smi-spec.io) that facilitates integration with pre-existing solution. 
Moreover, Traefik Mesh is opt-in by default, which means that your existing services are unaffected until you decide to add them to the mesh.

<p align="center">
<a href="https://smi-spec.io" target="_blank"><img width="150" src="docs/content/assets/img/smi.png" alt="SMI" title="SMI" /></a>
</p>


## Non-Invasive Service Mesh

Traefik Mesh does not use any sidecar container but handles routing through proxy endpoints running on each node. 
The mesh controller runs in a dedicated pod and handles all the configuration parsing and deployment to the proxy nodes. 
Traefik Mesh supports multiple configuration options: annotations on user service objects, and SMI objects. 
Not using sidecars means that Traefik Mesh does not modify your Kubernetes objects, and does not modify your traffic without your knowledge. 
Using the Traefik Mesh endpoints is all that is required.

<p align="center">
<img width="400" src="docs/content/assets/img/before-traefik-mesh-graphic.png" alt="Traefik Mesh" title="Traefik Mesh" />
<img width="400" src="docs/content/assets/img/after-traefik-mesh-graphic.png" alt="Traefik Mesh" title="Traefik Mesh" />
</p>

## Prerequisites

To run this app, you require the following:

- Kubernetes 1.11+
- CoreDNS installed as [Cluster DNS Provider](https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/) (versions 1.3+ supported)
- Helm v3

## Install (Helm v3 only)

```shell
helm repo add traefik-mesh https://traefik.github.io/mesh/charts
helm repo update
helm install traefik-mesh traefik-mesh/traefik-mesh
```

You can find the complete documentation at https://doc.traefik.io/mesh.


## Contributing

[Contributing guide](CONTRIBUTING.md).
