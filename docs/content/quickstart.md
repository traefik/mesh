# Quickstart

Traefik Mesh can be installed in your cluster without affecting any running services.
It can safely be installed using the [Helm Chart](https://helm.sh/docs/intro/using_helm/#helm-install-installing-a-package).

## Prerequisites

- Kubernetes 1.11+
- CoreDNS installed as [Cluster DNS Provider](https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/) (versions 1.3+ supported)
- Helm v3

### RBAC

Depending on the tool you used to deploy your cluster you might need to tweak RBAC permissions.

#### `kubeadm`

If you used `kubeadm` to deploy your cluster, a fast way to allow the helm installation to perform all steps it needs is to edit the
`cluster-admin` `ClusterRoleBinding`, adding the following to the `subjects` section:

```yaml
- kind: ServiceAccount
  name: default
  namespace: kube-system
```

## Installing Traefik Mesh

```bash tab="Command"
helm repo add traefik-mesh https://traefik.github.io/mesh/charts
helm repo update
helm install traefik-mesh traefik-mesh/traefik-mesh
```

```bash tab="Expected output"
[...]

NOTES:
Thank you for installing traefik-mesh.

Your release is named traefik-mesh.

To learn more about the release, try:

  $ helm status traefik-mesh
  $ helm get traefik-mesh
```

## Using Traefik Mesh

As an example, let's deploy a server application and a client application under the `test` namespace.

```yaml tab="server.yaml"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: server
  namespace: test
  labels:
    app: server
spec:
  replicas: 2
  selector:
    matchLabels:
      app: server
  template:
    metadata:
      labels:
        app: server
    spec:
      containers:
        - name: server
          image: traefik/whoami:latest
          ports:
            - containerPort: 80
---
kind: Service
apiVersion: v1
metadata:
  name: server
  namespace: test
spec:
  selector:
    app: server
  ports:
    - name: web
      protocol: TCP
      port: 80
      targetPort: 80
```

```yaml tab="client.yaml"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: client
  namespace: test
  labels:
    app: client
spec:
  replicas: 1
  selector:
    matchLabels:
      app: client
  template:
    metadata:
      labels:
        app: client
    spec:
      containers:
        - name: client
          image: giantswarm/tiny-tools:3.9
          imagePullPolicy: IfNotPresent
          command:
            - "sleep"
            - "infinity"
```

Create the namespace then deploy those two applications:

```bash
kubectl create namespace test
kubectl apply -f server.yaml
kubectl apply -f client.yaml
```

You should now see the following output:

```bash tab="Command"
kubectl get all -n test
```

```text tab="Expected output"
NAME                          READY     STATUS    RESTARTS   AGE
pod/client-7446fdf848-x96fq   1/1       Running   0          79s
pod/server-7c8fd58db5-rchg8   1/1       Running   0          77s
pod/server-7c8fd58db5-sd4f9   1/1       Running   0          77s

NAME             TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)   AGE
service/server   ClusterIP   10.43.17.247   <none>        80/TCP    77s

NAME                     READY     UP-TO-DATE   AVAILABLE   AGE
deployment.apps/client   1/1       1            1           79s
deployment.apps/server   2/2       2            2           77s

NAME                                DESIRED   CURRENT   READY     AGE
replicaset.apps/client-7446fdf848   1         1         1         79s
replicaset.apps/server-7c8fd58db5   2         2         2         77s
```

Take note of the client app pod name (here it's `client-7446fdf848-x96fq`) and open a new terminal session inside this pod using `kubectl exec`.

```bash
kubectl -n test exec -ti client-7446fdf848-x96fq ash
```

From inside the client container, make sure your server is reachable using the Kubernetes DNS service discovery.

```bash tab="Command"
curl server.test.svc.cluster.local
```

```test tab="Expected Output"
Hostname: server-7c8fd58db5-sd4f9
IP: 127.0.0.1
IP: ::1
IP: 10.42.2.10
IP: fe80::a4ec:77ff:fe37:1cdd
RemoteAddr: 10.42.2.9:46078
GET / HTTP/1.1
Host: server.test.svc.cluster.local
User-Agent: curl/7.64.0
Accept: */*
```

You can note that all this server application is doing is to respond with the content of the request it receives.

Now replace the `svc.cluster.local` suffix by `traefik.mesh`, and tada: you are now using Traefik Mesh to reach your server!

```bash tab="Command"
curl server.test.traefik.mesh
```

```test tab="Expected Output"
Hostname: server-7c8fd58db5-rchg8
IP: 127.0.0.1
IP: ::1
IP: 10.42.1.7
IP: fe80::601d:7cff:fe26:c8c6
RemoteAddr: 10.42.1.5:59478
GET / HTTP/1.1
Host: server.test.traefik.mesh
User-Agent: curl/7.64.0
Accept: */*
Accept-Encoding: gzip
Uber-Trace-Id: 3f9e7129a059f70:7e889a1ebcb147ac:3f9e7129a059f70:1
X-Forwarded-For: 10.42.2.9
X-Forwarded-Host: server.test.traefik.mesh
X-Forwarded-Port: 80
X-Forwarded-Proto: http
X-Forwarded-Server: traefik-mesh-proxy-w95q2
X-Real-Ip: 10.42.2.9
```

Note the presence of `X-Forwarded` headers as well as other instrumentation headers like `Uber-Trace-Id`, indicating than your request has been processed and instrumented by Traefik Mesh.

## What's next

See the [examples page](examples.md) to see a more advanced example, or dive into the [configuration](configuration.md) to discover all Traefik Mesh capabilities.
