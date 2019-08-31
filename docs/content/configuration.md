# Configuration

The configuration for maesh is broken into two parts: the static configuration, and the dynamic configuration. 
The static configuration is configured when the maesh service mesh is installed, 
and is configured via the values.yaml file in the helm install.

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

### Traffic type

Annotations on services are the main way to configure maesh behavior.

The service mode can be enabled by using the following annotation:

```yaml
maesh.containo.us/traffic-type: "http"
```

This annotation can be set to either `http` or `tcp`, and will specify the mode for that service operation.
If this annotation is not present, the mesh service will operate in the default mode specified in the static configuration.

### Retry

Retries can be enabled by using the following annotation:

```yaml
maesh.containo.us/retry-attempts: "2"
```

This annotation sets the number of retry attempts that maesh will make if a network error occurrs.
Please note that this value is a string, and needs to be quoted.

### Circuit breaker

Circuit breaker can be enabled by using the following annotation:

```yaml
maesh.containo.us/circuit-breaker-expression: "Expression"
```

This annotation sets the expression for circuit breaking. 
The circuit breaker protects your system from stacking requests to unhealthy services (resulting in cascading failures).
When your system is healthy, the circuit is close (normal operations). When your system becomes unhealthy, the circuit becomes open and the requests are no longer forwarded (but handled by a fallback mechanism).

All configuration options are available [here](https://docs.traefik.io/v2.0/middlewares/circuitbreaker/#configuration-options)
