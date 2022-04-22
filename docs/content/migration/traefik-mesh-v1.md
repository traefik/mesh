---
title: "Traefik Mesh Migrations Documentation"
description: "Traefik Mesh v1.4 introduced a few key changes. Learn what those changes are and what actionable steps must be taken in the technical documentation."
---

# Minor Migrations

Traefik Mesh v1
{: .subtitle }

## Traefik Mesh v1.4

Maesh has been renamed to Traefik Mesh in an effort to rename all of our products and make them look closer.
Through this renaming process, a couple of things changed which might be worth mentioning for a migration process.
All areas that changed are mentioned below with the appropriate actions needed to do.

Everything called Maesh references to `v1.3`, while Traefik Mesh refers to `v1.4`.

### Environment Variable Prefix

Prior to Traefik Mesh, environment variables were prefixed with `MAESH_`.
Now they're prefixed with `TRAEFIK_MESH_` and the `MAESH_` prefix is deprecated.
You need to decide on either using `MAESH_` or `TRAEFIK_MESH_` as mixing both will result in an error. 

### Configuration File Name

The default configuration file name is changed from `maesh` to `traefik-mesh` as well.

### DNS Name

The well known internal DNS name, to opt in into the usage of Maesh was `.maesh`.
Now, with the rebranding process this has been changed to `traefik.mesh` and thus, you now need to use the DNS name of `servicename.servicenamespace.traefik.maesh` to opt-in into the usage of Traefik Mesh.
The old name `.maesh`, is deprecated and will be removed eventually.

### Docker Image Name

As part of the process, the docker-image has been moved from `containous/maesh` to `traefik/mesh`.
The old image will not be updated anymore and `traefik/mesh` starts with `v1.4.0`.

### Binary

The new binary name is `traefik-mesh`, rather than `maesh` before.
However, as Traefik Mesh is running inside k8s this change should not be critical as it's hidden by the docker-image name.

### Annotations

As part of the rebranding process, the annotation prefix has changed. 
The annotation prefix `maesh.containo.us/` has been deprecated in favour of `mesh.traefik.io`.

## v1.1 to v1.2

### Debug

The `--debug` CLI flag is deprecated and will be removed in a future major release.
Instead, you should use the new `--logLevel` flag with `debug` as value.

### SMI Mode

The `--smi` CLI flag is deprecated and will be removed in a future major release.
Instead, you should use the new and backward compatible `--acl` flag.
