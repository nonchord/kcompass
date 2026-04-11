# Terraform modules for kcompass

Optional helpers for operators who manage cluster inventory with Terraform. These modules let you:

1. **Generate a cluster inventory file** and commit it into a GitHub repo that kcompass consumes via its git backend (`kcompass_inventory`).
2. **Format the TXT record value** that kcompass's auto-discovery looks for, so you can publish it with whichever DNS provider you use (`kcompass_txt`).

Both modules are optional — kcompass itself has no dependency on them, and operators are free to hand-write inventory YAML or publish TXT records directly. These are sugar for people already living in Terraform.

## Modules

- [`modules/kcompass_inventory`](modules/kcompass_inventory) — writes one inventory YAML file into an existing GitHub repo. Accepts full `ClusterRecord` entries (with either `kubeconfig.inline` or `kubeconfig.command`), matching the schema kcompass's git backend parses.
- [`modules/kcompass_txt`](modules/kcompass_txt) — pure value formatter. Takes a backend URL, returns `v=kc1; backend=<url>` plus the conventional record label (`kcompass`). No resources, no providers. Feed the outputs into whatever DNS provider you use.

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

See [`examples/basic`](examples/basic/main.tf) for a complete example that publishes three clusters (Tailscale, GKE, and an inline kubeconfig) and produces a TXT record value for publication.
