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
interval: 1m # default to '1m'

resource_version: v1 # default to 'v1'
resource: secrets

source:
  namespace: kube-ingress
  name: tls-cluster-wildcard
target:
  namespace: .+ # regex
```

## Example for Registry credentials replication

```yaml
```

## Credits

GUO YANKE, MIT License
