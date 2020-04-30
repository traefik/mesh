# Configuration

The configuration for Maesh is broken down into two parts: the static configuration, and the dynamic configuration.
The static configuration is configured when the service mesh is installed and is configured via the `values.yaml` file in the Helm install.

## Static configuration

- The Maesh image version can be manually defined if needed.

- Debug logging can be globally enabled.

- The default mesh mode can be configured. If this is not set, the default mode will be HTTP.
    This means that new mesh services that are not specified will default to operate in HTTP mode.

- Tracing can be enabled.

- Access-Control List (ACL) mode can be enabled.
    This configures Maesh to run in ACL mode, where all traffic is forbidden unless explicitly allowed via 
    an SMI [TrafficTarget](https://github.com/servicemeshinterface/smi-spec/blob/master/traffic-access-control.md).
    Please see the [SMI Specification](https://github.com/servicemeshinterface/smi-spec/blob/master/traffic-access-control.md) for more information.

## Dynamic configuration

Dynamic configuration can be provided to Maesh using annotations on Kubernetes services and via SMI objects. 

 | Features              | ACL disabled | ACL enabled |
 |-----------------------|--------------|-------------|
 | Traffic-Type          | ✔            | ✔           |
 | Scheme                | ✔            | ✔           |
 | Retry                 | ✔            | ✔           |
 | Circuit-Breaker       | ✔            | ✔           |
 | Rate-Limit            | ✔            | ✔           |
 | Traffic-Split (SMI)   | ✔            | ✔           |
 | Traffic-Target (SMI)  | ✘            | ✔           |

### Kubernetes Service Annotations

Annotations on services give the ability to configure how Maesh interprets them.

#### Traffic type

The traffic type can be configured by using the following annotation:

```yaml
maesh.containo.us/traffic-type: "http"
```

This annotation can be set to either `http`, `tcp` or `udp` and will specifies the mode for that service operation.
If this annotation is not present, the mesh service will operate in the default mode specified in the static configuration.

!!! Info
    For now, the `udp` traffic type does not work when ACL mode is enabled. In ACL mode, all traffic is forbidden unless it
    is explicitly allowed with a [TrafficTarget](https://github.com/servicemeshinterface/smi-spec/blob/master/traffic-access-control.md) and
    unfortunately the SMI specification does not yet define a [Traffic Spec](https://github.com/servicemeshinterface/smi-spec/blob/master/traffic-specs.md) for `UDP`.
    
#### Scheme

The scheme used to define custom scheme for request:

```yaml
maesh.containo.us/scheme: "h2c"
```

This annotation can be set to either `http`, `https` or `h2c` and is available for `maesh.containo.us/traffic-type: "http"`.

??? Note "Limitations"
    Please keep in mind, that if you set the scheme to `https` your service needs to expose itself via HTTPS as there is no
    mTLS in Maesh.

#### Retry

Retries can be enabled by using the following annotation:

```yaml
maesh.containo.us/retry-attempts: "2"
```

This annotation sets the number of retry attempts that Maesh will make if a network error occurs.
Please note that this value is a string, and needs to be quoted.

#### Circuit breaker

Circuit breaker can be enabled by using the following annotation:

```yaml
maesh.containo.us/circuit-breaker-expression: "Expression"
```

This annotation sets the expression for circuit breaking.
The circuit breaker protects your system from stacking requests to unhealthy services (resulting in cascading failures).
When your system is healthy, the circuit is closed (normal operations). When your system becomes unhealthy, the circuit opens, and requests are no longer forwarded (but handled by a fallback mechanism).

All configuration options are available [here](https://docs.traefik.io/v2.0/middlewares/circuitbreaker/#configuration-options).

#### Rate Limit

Rate limiting can be enabled by using the following annotations:

```yaml
maesh.containo.us/ratelimit-average: "100"
maesh.containo.us/ratelimit-burst: "200"
```

These annotation sets average and burst requests per second limit for the service.
Please note that this value is a string, and needs to be quoted.

Further details about the rate limiting can be found [here](https://docs.traefik.io/v2.0/middlewares/ratelimit/#configuration-options).

### Service Mesh Interface

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

- The `api` route under the `/api` path, accepting all methods.
- The `metrics` routes under the `/metrics` path, accepting only `GET` requests.

Other types of route groups and detailed information are available [in the specification](https://github.com/servicemeshinterface/smi-spec/blob/master/traffic-specs.md).

By default, all traffic is denied so we need to grant access to clients to our application. This is done by defining a `TrafficTarget`.

??? Note "TrafficTarget Source & Destination"
    Please note that TrafficTarget is a namespaced resource. Therefore, the source and the destination namespace needs to be explicitly defined.

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

Any client running with the service account `client` under the `client` namespace accessing `server.server.maesh/api` is allowed to access the `/api` resource. Others will receive 404 answers from the Maesh node.

More information can be found [in the SMI specification](https://github.com/servicemeshinterface/smi-spec/blob/master/traffic-access-control.md).

#### Traffic Splitting

SMI defines the `TrafficSplit` resource which allows to direct subsets of the traffic to different services.

```yaml
apiVersion: split.smi-spec.io/v1alpha2
kind: TrafficSplit
metadata:
  name: server-split
  namespace: server
spec:
  service: server
  backends:
  - service: server-v1
    weight: 80
  - service: server-v2
    weight: 20
```

In this example, we define a traffic split for our server service between two versions of our server, v1 and v2.
`server.server.maesh` directs 80% of the traffic to the server-v1 pods, and 20% of the traffic to the server-v2 pods.

More information can be found [in the SMI specification](https://github.com/servicemeshinterface/smi-spec/blob/master/traffic-split.md).

#### Traffic Metrics

At the moment, Maesh does not implement the [Traffic Metrics specification](https://github.com/servicemeshinterface/smi-spec/blob/master/traffic-metrics.md).
