# Configuration

The configuration for maesh is broken into two parts: the static configuration, and the dynamic configuration.
The static configuration is configured when the maesh service mesh is installed,
and is configured via the values.yaml file in the helm install.

## Static configuration

- The maesh image version can be manually defined if needed, as can the version for the Traefik CE mesh nodes.

- Debug logging can be globally enabled.

- The default mesh mode can be configured. If this is not set, the default mode will be HTTP.
    This means that new mesh services that are not specified will default to operate in HTTP mode.

- Tracing can be enabled.

- Service Mesh Interface (SMI) mode can be enabled.
    This configures maesh to run in SMI mode, where access and routes are explicitly enabled.
    Note: By default, all routes and access is denied.
    Please see the [SMI Specification](https://github.com/deislabs/smi-spec) for more information

## Dynamic configuration

Dynamic configuration can be provided to Maesh using either annotations on kubernetes services (default mode) or SMI resources if Maesh is installed with [SMI enabled](./install.md#service-mesh-interface).

### With Kubernetes Services

#### Traffic type

Annotations on services are the main way to configure maesh behavior.

The service mode can be enabled by using the following annotation:

```yaml
maesh.containo.us/traffic-type: "http"
```

This annotation can be set to either `http` or `tcp`, and will specify the mode for that service operation.
If this annotation is not present, the mesh service will operate in the default mode specified in the static configuration.

#### Scheme

The scheme used to define custom scheme for request:

```yaml
maesh.containo.us/scheme: "h2c"
```

This annotation can be set to either `http` or `h2c` and is available for `maesh.containo.us/traffic-type: "http"`.

#### Retry

Retries can be enabled by using the following annotation:

```yaml
maesh.containo.us/retry-attempts: "2"
```

This annotation sets the number of retry attempts that maesh will make if a network error occurrs.
Please note that this value is a string, and needs to be quoted.

#### Circuit breaker

Circuit breaker can be enabled by using the following annotation:

```yaml
maesh.containo.us/circuit-breaker-expression: "Expression"
```

This annotation sets the expression for circuit breaking.
The circuit breaker protects your system from stacking requests to unhealthy services (resulting in cascading failures).
When your system is healthy, the circuit is closed (normal operations). When your system becomes unhealthy, the circuit opens, and requests are no longer forwarded (but handled by a fallback mechanism).

All configuration options are available [here](https://docs.traefik.io/v2.0/middlewares/circuitbreaker/#configuration-options)

#### Rate Limit

Rate limiting can be enabled by using the following annotations:

```yaml
maesh.containo.us/ratelimit-average: "100"
maesh.containo.us/ratelimit-burst: "200"
```

These annotation sets average and burst requests per second limit for the service.
Please note that this value is a string, and needs to be quoted.

Further details about the rate limiting can be found [here](https://docs.traefik.io/v2.0/middlewares/ratelimit/#configuration-options)

### With Service Mesh Interface

#### Access Control

The first step is to describe what the traffic of our server application looks like.

```yaml
---
apiVersion: specs.smi-spec.io/v1alpha1
kind: HTTPRouteGroup
metadata:
  name: server-routes
  namespace: server
matches:
  - name: api
    pathRegex: /api
    methods: ["*"]
  - name: metrics
    pathRegex: /metrics
    methods: ["GET"]
```

In this example, we define a set of HTTP routes for our `server` application.

More precisely, the `server` app is composed by two routes:

- The `api` route under the `/api` path, accepting all methods
- The `metrics` routes under the `/metrics` path, accepting only a `GET` request

Other types of route groups and detailed information are available [in the specification](https://github.com/deislabs/smi-spec/blob/master/traffic-specs.md).

By default, all traffic is denied so we need to grant access to clients to our application. This is done by defining a `TrafficTarget`.

```yaml
---
apiVersion: access.smi-spec.io/v1alpha1
kind: TrafficTarget
metadata:
  name: client-server-target
  namespace: server
destination:
  kind: ServiceAccount
  name: server
  namespace: server
specs:
  - kind: HTTPRouteGroup
    name: server-routes
    matches:
      - api
sources:
  - kind: ServiceAccount
    name: client
    namespace: client
```

In this example, we grant access to all pods running with the service account `client` under the namespace `client` to the HTTP route `api` specified by on the group `server-routes` on all pods running with the service account `server` under the namespace `server`.

Any client running with the service account `client` under the `client` namespace accessing `server.server.maesh/api` is allowed to access the `/api` resource. Other will receive a 404 answer from the Maesh node.

More information can be found [in the specification](https://github.com/deislabs/smi-spec/blob/master/traffic-access-control.md).

#### Traffic Splitting

SMI defines the `TrafficSplit` resource which allows to direct incrementally a subset of the traffic to a different services.

```yaml
apiVersion: split.smi-spec.io/v1alpha1
kind: TrafficSplit
metadata:
  name: server-split
  namespace server
spec:
  service: server
  backends:
  - service: server-v1
    weight: 80
  - service: server-v2
    weight: 20
```

In this example, we define a traffic split for our server service between two version of our server, v1 and v2.
`server.server.maesh` directs 80% of the traffic to the server-v1 pods, and 20% of the traffic to the server-v2 pods.

More information can be found [in the specification](https://github.com/deislabs/smi-spec/blob/master/traffic-split.md).

#### Traffic Metrics

At the moment, Maesh does not implement the [Traffic Metrics specification](https://github.com/deislabs/smi-spec/blob/master/traffic-metrics.md).
