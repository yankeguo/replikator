# replikator

Kubernetes Resource Replicator

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

```yaml
# interval of execution, default to 1m
interval: 1m

# resource name, required, should be canonical plural, e.g. 'secrets', 'networking.k8s.io/v1/ingresses', 'apps/v1/deployments'
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
  # javascript code to modify the resource, optional, see below for details
  javascript: |
    resource.metadata.annotations["replikator/modified"] = new Date().toISOString()

  # jsonpatch to modify the resource, optional
  jsonpatch:
    - op: remove
      path: /metadata/annotations/replikator/modified

# multi-documents YAML are supported
# use --- to separate multiple tasks
---
# another task
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
    verbs: ["get", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["list"]
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
    interval: 1m
    resource_version: v1
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
