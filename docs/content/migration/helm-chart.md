# Migrations

Helm Chart
{: .subtitle }

## v1.x to v2.0

### SMI Mode

The `smi.enable` Helm option has been deprecated and removed. You should use the new and backward compatible ACL mode 
option as described in the [documentation](../install.md#access-control-list). 

### Docker image version

Since version `v1.2`, Maesh uses [Traefik](https://github.com/containous/traefik/) as a library and does not rely on its 
Docker image anymore. Therefore, the `controller.image` and `mesh.image` Helm options have been removed. You should use 
the new `image` option as described in the [documentation](../install.md#deploy-helm-chart).    
