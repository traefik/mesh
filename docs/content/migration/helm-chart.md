# Migrations

Helm Chart
{: .subtitle }

## v1.x to v2.0

### Image version

Since version `v1.2`, Maesh uses [Traefik](https://github.com/containous/traefik/) as a library and does not rely on its 
Docker image anymore. Therefore, the `controller.image` and `mesh.image` options have been removed. You should use the 
new `image` option as described in the [documentation](../install.md#deploy-helm-chart).    

### Log Level

The `controller.logging.debug` and `mesh.logging` options have been removed. You should use the new `controller.logLevel` 
and `mesh.logLevel` options to configure the logging level for the controller and proxies.

### SMI Mode

The `smi.enable` option has been deprecated and removed. You should use the new and backward compatible ACL mode 
option as described in the [documentation](../install.md#access-control-list). 
