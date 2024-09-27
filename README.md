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

`replikator` will watch the configuration directory for changes, and reload the configuration files.

```yaml
# resource name, required, should be canonical plural
# e.g. 'secrets', 'networking.k8s.io/v1/ingresses', 'apps/v1/deployments'
resource: secrets

# replication source
source:
  # source namespace, required
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
    resource.spec.ports.forEach(port => delete port.nodePort)
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
