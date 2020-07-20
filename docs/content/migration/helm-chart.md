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

## v2.0 to v2.1

### Default Mode

The `mesh.defaultMode` option has been deprecated and will be removed in a future major release.
You should use the new `defaultMode` option to configure the default traffic mode for Maesh services.

### Prometheus and Grafana services

Prior to version `v2.1`, when the Metrics chart is deployed, Prometheus and Grafana services are exposed by default through 
a `NodePort`. For security reasons, those services are not exposed by default anymore. To expose them you should use the 
new `prometheus.service` and `grafana.service` options, more details in the corresponding [values.yaml](https://github.com/containous/maesh/blob/e59b861ac91261b950663410a6223a02fc7e2290/helm/chart/maesh/charts/metrics/values.yaml).

## v2.1 to v3.0

### Default Mode

The `mesh.defaultMode` option has been removed. You should use the new `defaultMode` option to configure the default traffic 
mode for Maesh services.
