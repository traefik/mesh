# Quickstart

Maesh can be installed in your cluster without affecting any running services.
It will safely install itself via the helm chart, and will be ready for use immediately after.

It can be installed by running:

```shell
helm repo add maesh https://containous.github.io/maesh/charts
helm repo update
helm install --name=maesh --namespace=maesh maesh/maesh
```

## RBAC

Depending on the tool you used to deploy your cluster you might need
to tweak RBAC permissions.

### `kubeadm`

If you used `kubeadm` to deploy your cluster, a fast way to allow the
helm installation to perform all steps it needs is to edit the
`cluster-admin` `ClusterRoleBinding`, adding the following to the
`subjects` section:

```yaml
- kind: ServiceAccount
  name: default
  namespace: kube-system
```

Assuming `tiller` is deployed in your `kube-system` namespace, this will
give it very open permissions.
