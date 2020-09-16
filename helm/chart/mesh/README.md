# Traefik Mesh

Traefik Mesh is a simple, yet full-featured service mesh. It is container-native and fits as your de-facto service mesh in your Kubernetes cluster.
It supports the latest Service Mesh Interface specification [SMI](https://smi-spec.io/) that facilitates integration with pre-existing solution.

Moreover, Traefik Mesh is opt-in by default, which means that your existing services are unaffected until you decide to add them to the mesh.

## Prerequisites

- Kubernetes 1.11+
- CoreDNS/KubeDNS installed as [Cluster DNS Provider](https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/) (versions 1.3+ supported)
- [Helm v3](https://helm.sh/docs/intro/install/)

## Installing the Chart

To install the chart with the release name `traefik-mesh`:

```bash
$ helm repo add traefik-mesh https://traefik.github.io/mesh/charts
$ helm repo update
$ helm install traefik-mesh traefik-mesh/traefik-mesh
```

You can use the `--namespace my-namespace` flag to deploy Traefik Mesh in a custom namespace and the `--set "key1=val1,key2=val2,..."` flag to configure it.
Where `key1=val1`, `key2=val2`, `...` are chart values that you can find at https://github.com/traefik/mesh/blob/master/helm/chart/mesh/values.yaml.

## Uninstalling the Chart

To uninstall the chart with the release name `traefik-mesh`:

```bash
$ helm uninstall traefik-mesh
```

## Configuration

The following table lists the configurable parameters of the Traefik Mesh chart and their default values.

| Key                                            | Description                                                                                                                                                                                                                                                                     | Default                   |
|------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------|
| acl                                            | Enable ACL mode.                                                                                                                                                                                                                                                                | `false`                   |
| clusterDomain                                  | Custom cluster domain.                                                                                                                                                                                                                                                          | `"cluster.local"`         |
| controller.affinity                            | Node/Pod affinities for the controller.                                                                                                                                                                                                                                         | `{}`                      |
| controller.ignoreNamespaces                    | Namespace to ignore for the controller.                                                                                                                                                                                                                                         | `[]`                      |
| controller.image.name                          | Docker image for the controller.                                                                                                                                                                                                                                                | `"traefik/mesh"`          |
| controller.image.pullPolicy                    | Pull policy for the controller Docker image.                                                                                                                                                                                                                                    | `"IfNotPresent"`          |
| controller.image.pullSecret                    | Name of the Secret resource containing the private registry credentials for the controller image.                                                                                                                                                                               |                           |
| controller.image.tag                           | Tag for the controller container Docker image.                                                                                                                                                                                                                                  | `{{ .Chart.AppVersion }}` |
| controller.logFormat                           | Controller log format.                                                                                                                                                                                                                                                          | `"common"`                |
| controller.logLevel                            | Controller log level.                                                                                                                                                                                                                                                           | `"error"`                 |
| controller.nodeSelector                        | Node labels for pod assignment. See the [K8S documentation](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector) for more details.                                                                                                            | `{}`                      |
| controller.resources.limit.cpu                 | Maximum amount of CPU units that the controller container can use.                                                                                                                                                                                                              | `"200m"`                  |
| controller.resources.limit.mem                 | Maximum amount of memory that the controller container can use.                                                                                                                                                                                                                 | `"100Mi"`                 |
| controller.resources.request.cpu               | Amount of CPU units that the controller container requests.                                                                                                                                                                                                                     | `"100m"`                  |
| controller.resources.request.mem               | Amount of memory that the controller container requests.                                                                                                                                                                                                                        | `"50Mi"`                  |
| controller.tolerations                         | Tolerations section for the controller. See the [K8S documentation](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/) for more details.                                                                                                            | `[]`                      |
| controller.watchNamespaces                     | Namespace to watch for the controller.                                                                                                                                                                                                                                          | `[]`                      |
| defaultMode                                    | The default mesh mode. This means that new services will operate by default in HTTP mode.                                                                                                                                                                                       | `"http"`                  |
| kubedns                                        | Enable KubeDNS support.                                                                                                                                                                                                                                                         | `false`                   |
| limits.http                                    | Number of HTTP ports to allocate.                                                                                                                                                                                                                                               | `10`                      |
| limits.tcp                                     | Number of TCP ports to allocate.                                                                                                                                                                                                                                                | `25`                      |
| limits.udp                                     | Number of UDP ports to allocate.                                                                                                                                                                                                                                                | `25`                      |
| logFormat                                      | Log format for the controller and the proxy.                                                                                                                                                                                                                                    | `"common"`                |
| logLevel                                       | Log level for the controller and the proxy.                                                                                                                                                                                                                                     | `"error"`                 |
| proxy.additionalArguments                      | Arguments to be added to the proxy container args.                                                                                                                                                                                                                              | `[]`                      |
| proxy.annotations                              | Annotations to be added to the proxy deployment.                                                                                                                                                                                                                                | `{}`                      |
| proxy.env                                      | Additional environment variables to set in the proxy pods.                                                                                                                                                                                                                      | `[]`                      |
| proxy.envFrom                                  | Additional environment variables to set in the proxy pods.                                                                                                                                                                                                                      | `[]`                      |
| proxy.forwardingTimeouts.dialTimeout           | Maximum duration allowed for a connection to a backend server to be established. See the [Traefik documentation](https://docs.traefik.io/routing/overview/#forwardingtimeoutsdialtimeout) for more details.                                                                     | `"30s"`                   |
| proxy.forwardingTimeouts.idleConnTimeout       | Maximum amount of time an idle (keep-alive) connection will remain idle before closing itself. See the [Traefik documentation](https://docs.traefik.io/routing/overview/#forwardingtimeoutsresponseheadertimeout) for more details.                                             | `"1s"`                    |
| proxy.forwardingTimeouts.responseHeaderTimeout | Maximum amount of time, if non-zero, to wait for a server's response headers after fully writing the request (including its body, if any). See the [Traefik documentation](https://docs.traefik.io/routing/overview/#forwardingtimeoutsresponseheadertimeout) for more details. | `"0s"`                    |
| proxy.image.name                               | Docker image for the proxy.                                                                                                                                                                                                                                                     | `"traefik"`               |
| proxy.image.pullPolicy                         | Pull policy for the proxy image.                                                                                                                                                                                                                                                | `"IfNotPresent"`          |
| proxy.image.pullSecret                         | Name of the Secret resource containing the private registry credentials for the proxy image.                                                                                                                                                                                    |                           |
| proxy.image.tag                                | Tag for the proxy container Docker image.                                                                                                                                                                                                                                       | `"v2.3"`                  |
| proxy.logFormat                                | Proxy log format.                                                                                                                                                                                                                                                               | `"common"`                |
| proxy.logLevel                                 | Proxy log level.                                                                                                                                                                                                                                                                | `"error"`                 |
| proxy.nodeSelector                             | Node labels for pod assignment. See the [K8S documentation](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector) for more details.                                                                                                            | `{}`                      |
| proxy.podAnnotations                           | Annotations to be added to the proxy pods.                                                                                                                                                                                                                                      | `{}`                      |
| proxy.pollInterval                             | Polling interval to get the configuration from the controller.                                                                                                                                                                                                                  | `"1s"`                    |
| proxy.pollTimeout                              | Polling timeout when connecting to the controller configuration endpoint.                                                                                                                                                                                                       | `"1s"`                    |
| proxy.resources.limit.cpu                      | Maximum amount of CPU units that the proxy container can use.                                                                                                                                                                                                                   | `"200m"`                  |
| proxy.resources.limit.mem                      | Maximum amount of memory that the proxy container can use.                                                                                                                                                                                                                      | `"100Mi"`                 |
| proxy.resources.request.cpu                    | Amount of CPU units that the proxy container requests.                                                                                                                                                                                                                          | `"100m"`                  |
| proxy.resources.request.mem                    | Amount of memory that the proxy container requests.                                                                                                                                                                                                                             | `"50Mi"`                  |
| proxy.tolerations                              | Tolerations section for the proxy. See the [K8S documentation](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/) for more details.                                                                                                                 | `[]`                      |

### Metrics

The following table lists the available parameters to configure the metrics integrations on the Traefik Mesh proxies.
As the proxies are vanilla Traefik, check out the corresponding [documentation](https://docs.traefik.io/observability/metrics/overview/) for more details.

| Key                                              | Description                                                                                                      | Default                                    |
|--------------------------------------------------|------------------------------------------------------------------------------------------------------------------|--------------------------------------------|
| metrics.datadog.addEntrypointsLabels             | Enable metrics on entry points.                                                                                  | `true`                                     |
| metrics.datadog.addServiceLabels                 | Enable metrics on services.                                                                                      | `true`                                     |
| metrics.datadog.address                          | Address of the `datadog-agent` to send metrics to.                                                               | `"127.0.0.1:8125"`                         |
| metrics.datadog.pushInterval                     | Interval used by the exporter to push metrics to the `datadog-agent`.                                            | `"10s"`                                    |
| metrics.deploy                                   | Deploy the metric chart which contains [Grafana](https://grafana.com/) and [Prometheus](https://prometheus.io/). | `true`                                     |
| metrics.influxdb.addEntrypointsLabels            | Enable metrics on entry points.                                                                                  | `true`                                     |
| metrics.influxdb.addServiceLabels                | Enable metrics on services.                                                                                      | `true`                                     |
| metrics.influxdb.address                         | Address of the `InfluxDB` to send metrics to.                                                                    | `"localhost:8089"`                         |
| metrics.influxdb.database                        | Database to use when the protocol is `HTTP`.                                                                     |                                            |
| metrics.influxdb.password                        | Password, only for the `HTTP` protocol.                                                                          |                                            |
| metrics.influxdb.protocol                        | Address protocol, `udp` or `http`.                                                                               | `"udp"`                                    |
| metrics.influxdb.pushInterval                    | Interval used by the exporter to push metrics.                                                                   | `"10s"`                                    |
| metrics.influxdb.retentionPolicy                 | Retention policy used when the protocol is `HTTP`.                                                               |                                            |
| metrics.influxdb.username                        | Username, only for the `HTTP` protocol.                                                                          |                                            |
| metrics.prometheus.addEntrypointsLabels          | Enable metrics on entry points.                                                                                  | `true`                                     |
| metrics.prometheus.addServiceLabels              | Enable metrics on services.                                                                                      | `true`                                     |
| metrics.prometheus.buckets                       | Buckets for latency metrics.                                                                                     | `"0.100000, 0.300000, 1.200000, 5.000000"` |
| metrics.prometheus.grafana.resources.limit.cpu   | Maximum amount of CPU units that the Grafana container can use.                                                  | `"500m"`                                   |
| metrics.prometheus.grafana.resources.limit.mem   | Maximum amount of memory that the Grafana container can use.                                                     | `"500Mi"`                                  |
| metrics.prometheus.grafana.resources.request.cpu | Amount of CPU units that the Grafana container requests.                                                         | `"200m"`                                   |
| metrics.prometheus.grafana.resources.request.mem | Amount of memory that the Grafana container requests.                                                            | `"200Mi"`                                  |
| metrics.prometheus.grafana.storageClassName      | Storage class. See the [K8S documentation](https://kubernetes.io/docs/concepts/storage/storage-classes/)         | `"metrics-storage"`                        |
| metrics.statsd.addEntrypointsLabels              | Enable metrics on entry points.                                                                                  | `true`                                     |
| metrics.statsd.addServiceLabels                  | Enable metrics on services.                                                                                      | `true`                                     |
| metrics.statsd.address                           | Instructs the exporter to send metrics to `statsd` at this address.                                              | `"127.0.0.1:8125"`                         |
| metrics.statsd.prefix                            | Prefix to use for metrics collection.                                                                            | `"traefik"`                                |
| metrics.statsd.pushInterval                      | Interval used by the exporter to push the metrics to `statsd`.                                                   | `"10s"`                                    |

### Tracing

The following table lists the available parameters to configure the tracing integrations on the Traefik Mesh proxies.
As the proxies are vanilla Traefik, check out the corresponding [documentation](https://docs.traefik.io/observability/tracing/overview/) for more details.   

| Key                                      | Description                                                                                                                                      | Default                                |
|------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------|
| tracing.datadog.debug                    | Enable Datadog debug.                                                                                                                            | `false`                                |
| tracing.datadog.globalTag                | Apply shared tag in a form of `Key:Value` to all traces.                                                                                         | `""`                                   |
| tracing.datadog.localagenthostport       | Address of the `datadog-tracing-agent` to send the spans to.                                                                                     | `"127.0.0.1:8126"`                     |
| tracing.datadog.prioritySampling         | Enable priority sampling. When using distributed tracing, this option must be enabled in order to get all parts of a distributed trace sampled.. | `false`                                |
| tracing.deploy                           | Deploy the tracing sub-chart which contains [Jaeger](https://www.jaegertracing.io/).                                                             | `true`                                 |
| tracing.haystack.baggagePrefixHeaderName | Specifies the header name prefix that will be used to store baggage items in a map.                                                              | `""`                                   |
| tracing.haystack.globalTag               | Apply shared tag in a form of `Key:Value` to all the traces.                                                                                     | `""`                                   |
| tracing.haystack.localAgentHost          | Host of the `haystack-agent` to send spans to.                                                                                                   | `"127.0.0.1"`                          |
| tracing.haystack.localAgentPort          | Port of the `haystack-agent` to send spans to.                                                                                                   | `35000`                                |
| tracing.haystack.parentIDHeaderName      | Specifies the header name that will be used to store the parent ID.                                                                              | `""`                                   |
| tracing.haystack.spanIDHeaderName        | Specifies the header name that will be used to store the span ID.                                                                                | `""`                                   |
| tracing.haystack.traceIDHeaderName       | Specifies the header name that will be used to store the trace ID.                                                                               | `""`                                   |
| tracing.instana.localAgentHost           | Host of the `instana-agent` to send spans to.                                                                                                    | `"127.0.0.1"`                          |
| tracing.instana.localAgentPort           | Port of the `instana-agent` to send spans to.                                                                                                    | `42699`                                |
| tracing.instana.logLevel                 | Log Level.                                                                                                                                       | `"info"`                               |
| tracing.jaeger.enabled                   | Enable the Jaeger integration.                                                                                                                   | `true`                                 |
| tracing.jaeger.localagenthostport        | Host and Port of the `jaeger-agent` to send spans to.                                                                                            | `"127.0.0.1:6831"`                     |
| tracing.jaeger.samplingserverurl         | Address of the jaeger-agent's `HTTP` sampling server.                                                                                            | `"http://localhost:5778/sampling"`     |
| tracing.zipkin.httpEndpoint              | Instructs the exporter to send metrics to `ZipKin` at this address.                                                                              | `"http://localhost:9411/api/v2/spans"` |
| tracing.zipkin.id128Bit                  | Use 128 bit trace IDs.                                                                                                                           | `true`                                 |
| tracing.zipkin.sameSpan                  | Use SameSpan RPC style traces.                                                                                                                   | `false`                                |
| tracing.zipkin.sampleRate                | Rate between 0.0 and 1.0 of requests to trace.                                                                                                   | `1.0`                                  |

## Contributing

If you want to contribute to this chart, please read the [Guidelines](./Guidelines.md).
