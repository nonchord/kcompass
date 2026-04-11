# kcompass_inventory

Writes a single cluster inventory YAML file into an existing GitHub repository. kcompass's git backend walks the repo path and parses every `*.yaml` file with a top-level `clusters:` key, so multiple callers can write to the same repo (one file per environment) and kcompass will merge them.

This module does **not** create the repository. Create it once with `github_repository` (or any equivalent), then point one or more `kcompass_inventory` calls at it.

## Usage

```hcl
module "staging_inventory" {
  source = "git::https://github.com/nonchord/kcompass.git//terraform/modules/kcompass_inventory?ref=<tag>"

  repository = "clusters"
  filename   = "staging.yaml"

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
| `repository` | string | — | Existing GitHub repository name under the provider's configured owner. |
| `filename` | string | — | Path within the repo where this inventory file is written. Each caller writing to the same repo must use a distinct filename. |
| `branch` | string | `null` (repo default) | Branch to commit on. |
| `commit_message` | string | `"chore(kcompass): update cluster inventory"` | Commit message. |
| `clusters` | list(object) | — | See schema above. |

## Outputs

| Name | Description |
|---|---|
| `filename` | The file path written inside the inventory repository. |
| `rendered_yaml` | The full YAML document committed. Useful for local preview or tests. |

## Provider requirements

This module requires the `integrations/github` provider (≥ 6.0) to be configured in the calling module with appropriate credentials. It creates one `github_repository_file` resource per invocation.
