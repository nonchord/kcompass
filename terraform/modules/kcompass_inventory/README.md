# kcompass_inventory

Renders a kcompass cluster inventory YAML document from a list of `ClusterRecord` entries. The output is a single string — feed it into whatever mechanism your team uses to commit files to a git repository (GitHub, GitLab, Bitbucket, local file, CI pipeline, etc.).

This module has **no resources and no provider dependencies**. It's a pure value transform — same pattern as `kcompass_txt`.

kcompass's git backend walks the repo path and parses every `*.yaml` file with a top-level `clusters:` key, so multiple callers can render to the same repo (one file per environment) and kcompass will merge them.

## Usage

### With GitHub

```hcl
module "inventory" {
  source = "git::https://github.com/nonchord/kcompass.git//terraform/modules/kcompass_inventory?ref=<tag>"

  clusters = [
    {
      name        = "staging"
      description = "Staging cluster reachable via Tailscale"
      labels      = { env = "staging" }
      kubeconfig = {
        command = ["tailscale", "configure", "kubeconfig", "staging"]
      }
    },
  ]
}

resource "github_repository_file" "inventory" {
  repository          = "clusters"
  file                = "staging.yaml"
  content             = module.inventory.rendered_yaml
  commit_message      = "chore(kcompass): update cluster inventory"
  overwrite_on_create = true
}
```

### With GitLab

```hcl
resource "gitlab_repository_file" "inventory" {
  project        = "my-group/clusters"
  file_path      = "staging.yaml"
  branch         = "main"
  content        = base64encode(module.inventory.rendered_yaml)
  commit_message = "chore(kcompass): update cluster inventory"
}
```

### With a local file (for air-gapped / manual workflows)

```hcl
resource "local_file" "inventory" {
  filename = "${path.root}/clusters/staging.yaml"
  content  = module.inventory.rendered_yaml
}
```

## Cluster record schema

Each entry in `clusters` mirrors kcompass's `ClusterRecord`. Required: `name`, and exactly one of `kubeconfig.inline` or `kubeconfig.command`.

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | Used as the cluster name in `kcompass list` / `kcompass connect`. |
| `description` | string | no | Shown in `kcompass list`. |
| `labels` | map(string) | no | Opaque metadata — kcompass exposes them but doesn't interpret. |
| `kubeconfig.inline` | string | one-of | Complete kubeconfig YAML document, shipped with the record and merged as-is. Use when the same kubeconfig works for every consumer (OIDC exec plugins, service account tokens behind a VPN). |
| `kubeconfig.command` | list(string) | one-of | argv vector kcompass runs at connect time to mint a per-user kubeconfig. The command must honor `KUBECONFIG` and write to the file it points at. Examples: `["tailscale", "configure", "kubeconfig", "my-cluster"]`, `["gcloud", "container", "clusters", "get-credentials", ...]`, `["aws", "eks", "update-kubeconfig", ...]`. |

The module's variable validation rejects records that set neither or both of `inline`/`command`.

## Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `clusters` | list(object) | — | See schema above. |

## Outputs

| Name | Description |
|---|---|
| `rendered_yaml` | The full YAML document. Feed this into a git provider resource or a local file. |

## Provider requirements

None. This module is a pure value transform with no resources.
