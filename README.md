# replikator

[![codecov](https://codecov.io/gh/yankeguo/replikator/graph/badge.svg?token=J7KQ5P4WPF)](https://codecov.io/gh/yankeguo/replikator)

A kubernetes resource replicator.

`replikator` watches a resource in a namespace, and replicates it to other namespaces.

## Usage

```bash
replikator --conf CONFIG_DIR --kubeconfig path/to/kubeconfig
```

## Container Image

```
yankeguo/replikator
ghcr.io/yankeguo/replikator
```

**Mount the kubeconfig file to `/root/.kube/config`, or setup RBAC for in-cluster authentication**

**Mount configuration files to `/replikator`**

## Configuration File

`replikator` polls the configuration directory every 10 seconds and reloads it when a file changes. Polling is used because a mounted ConfigMap does not reliably emit filesystem events. If the new files fail to parse, the tasks already running are left in place.

## Behavior

- The source namespace is never a replication target, even when `target.namespace` matches it. Namespaces that are terminating are skipped.
- Replicas are written with server-side apply and forced to match the transformed source. The field manager is `io.github.yankeguo/replikator`.
- Each replica is annotated with `replikator.yankeguo.github.io/managed`, `source-namespace`, and `source-name`. When the source object is deleted, only replicas carrying those annotations are deleted.
- An unchanged source is not written again. A full resync runs every 10 minutes, and again whenever a watch reconnects, so a deleted replica is created again.
- Inside a cluster, `source.namespace` may be omitted. It then defaults to the pod namespace.

Modifications run before volatile metadata is removed, so a JSON patch can still target fields such as `/status` or `/spec/clusterIP`. `metadata.resourceVersion`, `uid`, `managedFields`, `ownerReferences`, and `status` are not copied.

```yaml
# resource name, required, canonical plural
# e.g. 'secrets', 'networking.k8s.io/v1/ingresses', 'apps/v1/deployments'
resource: secrets

# replication source
source:
  # source namespace; optional in-cluster, where it defaults to the pod namespace
  namespace: kube-ingress
  # source resource name, required
  name: tls-cluster-wildcard

# replication target
target:
  # target namespace regexp, required
  namespace: .+
  # target resource name, optional, default to source name
  name: "tls-cluster-wildcard"

# modification of the resource, optional
modification:
  # jsonpatch to modify the resource, optional
  jsonpatch:
    - op: remove
      path: /metadata/annotations/remove-this

  # javascript code to modify the resource, optional, see below for details
  javascript: |
    resource.metadata.annotations = resource.metadata.annotations || {}
    resource.metadata.annotations["replikator/modified"] = new Date().toISOString()


# multi-documents YAML are supported
# use --- to separate multiple tasks
---
# another task
```

## Modification

### JSONPatch

A list of JSONPatch operations to modify the resource.

A example to remove `spec.clusterIP` and `spec.clusterIPs` from a `Service` resource.

```yaml
modification:
  jsonpatch:
    - op: remove
      path: /spec/clusterIP
    - op: remove
      path: /spec/clusterIPs
```

### JavaScript

You can use JavaScript to modify the resource, just modify the `resource` object in place.

A example to remove `spec.ports[*].nodePort` from a `Service` resource.

```yaml
modification:
  javascript: |
    var ports = resource.spec.ports || []
    ports.forEach(function (port) {
      delete port.nodePort
    })
```

## Examples

### In-Cluster Registry Credentials Replication

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: replikator
automountServiceAccountToken: true
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: replikator
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "create", "update", "patch", "watch"]
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: replikator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: replikator
subjects:
  - kind: ServiceAccount
    name: replikator
    namespace: default
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: replikator-config
data:
  replikator.yaml: |
    resource: secrets
    source:
      namespace: default
      name: registry-credentials
    target:
      namespace: .+
---
apiVersion: v1
kind: Service
metadata:
  name: replikator
spec:
  clusterIP: None
  selector:
    app: replikator
  ports:
    - protocol: TCP
      port: 42
      name: placeholder
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: replikator
spec:
  replicas: 1
  serviceName: replikator
  selector:
    matchLabels:
      app: replikator
  template:
    metadata:
      labels:
        app: replikator
    spec:
      serviceAccountName: replikator
      volumes:
        - name: replikator-config
          configMap:
            name: replikator-config
      containers:
        - name: replikator
          image: yankeguo/replikator
          imagePullPolicy: Always
          volumeMounts:
            - name: replikator-config
              mountPath: /replikator
```

## Credits

GUO YANKE, MIT License
