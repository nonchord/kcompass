# Terraform modules for kcompass

Optional helpers for operators who manage cluster inventory with Terraform. These modules let you:

1. **Render a cluster inventory YAML document** from a list of `ClusterRecord` entries, ready to commit to whatever git provider you use (`kcompass_inventory`).
2. **Format the TXT record value** that kcompass's auto-discovery looks for, so you can publish it with whichever DNS provider you use (`kcompass_txt`).

Both modules are **pure value transforms** — no resources, no provider dependencies. They output strings; you feed those strings into your provider-specific resources (GitHub, GitLab, Cloudflare, Route 53, local files, etc.). kcompass itself has no dependency on them; they're sugar for people already living in Terraform.

## Modules

- [`modules/kcompass_inventory`](modules/kcompass_inventory) — renders a kcompass inventory YAML document from a list of cluster records. Accepts full `ClusterRecord` entries (with either `kubeconfig.inline` or `kubeconfig.command`), matching the schema kcompass's git backend parses. No resources, no providers.
- [`modules/kcompass_txt`](modules/kcompass_txt) — renders the `v=kc1; backend=<url>` TXT record value plus the conventional record label (`kcompass`). No resources, no providers.

## Usage from outside this repo

Source modules directly over git from a tagged release:

```hcl
module "inventory" {
  source = "git::https://github.com/nonchord/kcompass.git//terraform/modules/kcompass_inventory?ref=<tag>"
  # ...
}
```

The module version is coupled to the kcompass tool version, so pinning a tag pins the inventory schema your operators are authoring against.

## Example

See [`examples/basic`](examples/basic/main.tf) for a complete example that renders three clusters (Tailscale, GKE, and an inline kubeconfig), commits the YAML to a GitHub repo, and produces a TXT record value for publication. The example shows the split between the provider-agnostic renderer and the provider-specific `github_repository_file` resource.
