---
title: "Traefik Mesh Documentation"
description: "View some simple and ACL examples for how to deploy Traefik Mesh, a simple and lightweight service mesh, in your cluster. Read the technical documentation."
---

# Examples

Here are some examples on how to easily deploy Traefik Mesh on your cluster.

??? Note "Prerequisites"
    Before following those examples, make sure your cluster follows [the prerequisites for deploying Traefik Mesh](quickstart.md#prerequisites).

## Simple Example

Deploy those two yaml files on your Kubernetes cluster in order to add a simple backend example, available through HTTP and TCP.

```yaml tab="namespace.yaml"
apiVersion: v1
kind: Namespace
metadata:
  name: whoami

---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: whoami-server
  namespace: whoami

---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: whoami-client
  namespace: whoami
```

```yaml tab="deployment.yaml"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: whoami
  namespace: whoami
spec:
  replicas: 2
  selector:
    matchLabels:
      app: whoami
  template:
    metadata:
      labels:
        app: whoami
    spec:
      serviceAccount: whoami-server
      containers:
        - name: whoami
          image: traefik/whoami:v1.10.1
          imagePullPolicy: IfNotPresent

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: whoami-tcp
  namespace: whoami
spec:
  replicas: 2
  selector:
    matchLabels:
      app: whoami-tcp
  template:
    metadata:
      labels:
        app: whoami-tcp
    spec:
      serviceAccount: whoami-server
      containers:
        - name: whoami-tcp
          image: traefik/whoamitcp:v0.3.0
          imagePullPolicy: IfNotPresent

---
apiVersion: v1
kind: Service
metadata:
  name: whoami
  namespace: whoami
  labels:
    app: whoami
spec:
  type: ClusterIP
  ports:
    - port: 80
      name: whoami
  selector:
    app: whoami

---
apiVersion: v1
kind: Service
metadata:
  name: whoami-tcp
  namespace: whoami
  labels:
    app: whoami-tcp
spec:
  type: ClusterIP
  ports:
    - port: 8080
      name: whoami-tcp
  selector:
    app: whoami-tcp

---
apiVersion: v1
kind: Pod
metadata:
  name: whoami-client
  namespace: whoami
spec:
  serviceAccountName: whoami-client
  containers:
    - name: whoami-client
      image: giantswarm/tiny-tools:3.12
      command:
        - "sleep"
        - "3600"
```

You should now see the following when running `kubectl get all -n whoami`:

```text
NAME                             READY   STATUS    RESTARTS   AGE
pod/whoami-client                1/1     Running   0          11s
pod/whoami-f4cbd7f9c-lddgq       1/1     Running   0          12s
pod/whoami-f4cbd7f9c-zk4rb       1/1     Running   0          12s
pod/whoami-tcp-7679bc465-ldlt2   1/1     Running   0          12s
pod/whoami-tcp-7679bc465-wf87n   1/1     Running   0          12s

NAME                 TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
service/whoami       ClusterIP   100.68.109.244   <none>        80/TCP     13s
service/whoami-tcp   ClusterIP   100.68.73.211    <none>        8080/TCP   13s

NAME                         DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/whoami       2         2         2            2           13s
deployment.apps/whoami-tcp   2         2         2            2           13s

NAME                                   DESIRED   CURRENT   READY   AGE
replicaset.apps/whoami-f4cbd7f9c       2         2         2       13s
replicaset.apps/whoami-tcp-7679bc465   2         2         2       13s
```

You should now be able to make direct requests on your `whoami` service through HTTP.

```bash tab="Command"
kubectl -n whoami exec whoami-client -- curl -s whoami.whoami.svc.cluster.local
```

```text tab="Expected Output"
Hostname: whoami-84bdf87956-gvbm8
IP: 127.0.0.1
IP: 5.6.7.8
RemoteAddr: 1.2.3.4:12345
GET / HTTP/1.1
Host: whoami.whoami.svc.cluster.local
User-Agent: curl/7.69.1
Accept: */*
```

And through TCP, by executing the following `netcat` command and sending some data.

```bash tab="Command"
kubectl -n whoami exec -ti whoami-client -- nc whoami-tcp.whoami.svc.cluster.local 8080
my data
```

```text tab="Expected Output"
Received: my data
```

You can now install Traefik Mesh [by following this documentation](install.md) on your cluster.

Since Traefik Mesh is not intrusive, it has to be explicitly given access to services before it can be used. You can ensure that the HTTP endpoint of your service does not pass through Traefik Mesh since no `X-Forwarded-For` header should be added.

Now, in order to configure Traefik Mesh for your `whoami` service, you just need to update the `whoami` service specs, in order to add the appropriate annotations.

The HTTP service needs to have `mesh.traefik.io/traffic-type: "http"` and the TCP service, `mesh.traefik.io/traffic-type: "tcp"`.

```yaml
---
apiVersion: v1
kind: Service
metadata:
  name: whoami
  namespace: whoami
  labels:
    app: whoami
  annotations:
    mesh.traefik.io/traffic-type: "http"
    mesh.traefik.io/retry-attempts: "2"
spec:
  type: ClusterIP
  ports:
    - port: 80
      name: whoami
  selector:
    app: whoami

---
apiVersion: v1
kind: Service
metadata:
  name: whoami-tcp
  namespace: whoami
  labels:
    app: whoami-tcp
  annotations:
    mesh.traefik.io/traffic-type: "tcp"
spec:
  type: ClusterIP
  ports:
    - port: 8080
      name: whoami-tcp
  selector:
    app: whoami-tcp
```

You should now be able to access your HTTP and TCP services through the Traefik Mesh endpoint:

```bash tab="Command"
kubectl -n whoami exec whoami-client -- curl -s whoami.whoami.traefik.mesh
```

```text tab="Expected Output"
Hostname: whoami-84bdf87956-gvbm8
IP: 127.0.0.1
IP: 5.6.7.8
RemoteAddr: 1.2.3.4:12345
GET / HTTP/1.1
Host: whoami.whoami.traefik.mesh
User-Agent: curl/7.69.1
Accept: */*
X-Forwarded-For: 3.4.5.6
```

## ACL Example

The [ACL mode](install.md#access-control-list) can be enabled when installing Traefik Mesh.
Once activated, all traffic is forbidden unless explicitly authorized using the SMI [TrafficTarget](https://github.com/servicemeshinterface/smi-spec/blob/main/apis/traffic-access/v1alpha2/traffic-access.md#traffictarget) resource.
This example will present the configuration required to allow the client pod to send traffic to the HTTP and TCP services defined in the previous example.

Each `TrafficTarget` defines that a set of source `ServiceAccount` is capable of sending traffic to a destination `ServiceAccount`.
To authorize the `whoami-client` pod to send traffic to `whoami.whoami.traefik.mesh`, we need to explicitly allow it to hit the pods exposed by the `whoami` service. 

```yaml
---
apiVersion: specs.smi-spec.io/v1alpha3
kind: HTTPRouteGroup
metadata:
  name: http-everything
  namespace: whoami
spec:
  matches:
    - name: everything
      pathRegex: ".*"
      methods: ["*"]

---
apiVersion: access.smi-spec.io/v1alpha2
kind: TrafficTarget
metadata:
  name: whatever
  namespace: whoami
spec:
  destination:
    kind: ServiceAccount
    name: whoami-server
    namespace: whoami
    port: 80
  rules:
    - kind: HTTPRouteGroup
      name: http-everything
      matches:
        - everything
  sources:
    - kind: ServiceAccount
      name: whoami-client
      namespace: whoami
```

Incoming traffic on a TCP service can also be authorized using a `TCPRoute` and a `TrafficTarget`.

```yaml
---
apiVersion: specs.smi-spec.io/v1alpha3
kind: TCPRoute
metadata:
  name: my-tcp-route
  namespace: whoami
spec: {}

---
apiVersion: access.smi-spec.io/v1alpha2
kind: TrafficTarget
metadata:
  name: api-service-target
  namespace: whoami
spec:
  destination:
    kind: ServiceAccount
    name: whoami-server
    namespace: whoami
  rules:
    - kind: TCPRoute
      name: my-tcp-route
  sources:
    - kind: ServiceAccount
      name: whoami-client
      namespace: whoami
```
