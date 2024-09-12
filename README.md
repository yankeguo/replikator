# replikator

Kubernetes Resource Replicator

## Usage

```bash
replikator --conf CONFIG_DIR --kubeconfig path/to/kubeconfig
```

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

## Credits

GUO YANKE, MIT License
