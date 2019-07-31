# Configuration

The configuration for maesh is broken into two parts: the static configuration, and the dynamic configuration. The static configuration is configured when the maesh service mesh is installed, and is configured via the values.yaml file in the helm install.

## Static configuration

- The maesh image version can be manually defined if needed, as can the version for the Traefik CE mesh nodes.

- Debug logging can be globally enabled.

- The default mesh node can be configured. If this is not set, the default mode will be HTTP.
    This means that new mesh services that are not specified will default to operate in HTTP mode.

- Tracing can be enabled.

- Service Mesh Interface (SMI) mode can be enabled.
    This configures maesh to run in SMI mode, where access and routes are explicitly enabled.
    Note: By default, all routes and access is denied.
    Please see the [SMI Specification](https://github.com/deislabs/smi-spec) for more information

## Dynamic configuration

Annotations on services are the main way to configure maesh behavior.

The service mode can be enabled by using the following annotation:

```shell
maesh.containo.us/maesh-traffic-type: http
```

This annotation can be set to either `http` or `tcp`, and will specify the mode for that service operation.
If this annotation is not present, the mesh service will operate in the default mode specified in the static configuration.
