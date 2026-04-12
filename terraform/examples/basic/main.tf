terraform {
  required_version = ">= 1.3.0"

  required_providers {
    github = {
      source  = "integrations/github"
      version = ">= 6.0"
    }
  }
}

provider "github" {
  owner = "example-org"
}

# kcompass_inventory is a pure renderer — it takes a list of cluster records
# and outputs a YAML document. No resources, no provider dependency. The
# github_repository_file below is the provider-specific part that commits
# the rendered YAML into a GitHub repo. For GitLab, Bitbucket, or local
# files, swap that resource for your provider's equivalent.
module "inventory" {
  source = "../../modules/kcompass_inventory"

  clusters = [
    # A Tailscale-operator cluster: `tailscale configure kubeconfig` mints
    # per-user credentials at connect time.
    {
      name        = "staging"
      description = "Staging cluster reachable via Tailscale"
      labels      = { env = "staging" }
      kubeconfig = {
        command = ["tailscale", "configure", "kubeconfig", "staging"]
      }
    },

    # A GKE cluster: `gcloud` mints per-user credentials.
    {
      name        = "prod-gke"
      description = "Production GKE cluster"
      labels      = { env = "prod", provider = "gke" }
      kubeconfig = {
        command = ["gcloud", "container", "clusters", "get-credentials", "prod", "--region", "us-central1"]
      }
    },

    # An inline kubeconfig for a shared dev cluster — same kubeconfig for everyone.
    {
      name        = "dev-shared"
      description = "Shared dev k3s with an OIDC exec plugin"
      kubeconfig = {
        inline = <<-KCFG
          apiVersion: v1
          kind: Config
          clusters:
            - name: dev-shared
              cluster:
                server: https://dev.example-org.internal:6443
          contexts:
            - name: dev-shared
              context:
                cluster: dev-shared
                user: dev-shared
          current-context: dev-shared
          users:
            - name: dev-shared
              user:
                exec:
                  apiVersion: client.authentication.k8s.io/v1
                  command: kubectl-oidc_login
                  args: [get-token]
        KCFG
      }
    },
  ]
}

# Commit the rendered inventory into a GitHub repo. This is the only
# provider-specific part — swap for gitlab_repository_file, local_file,
# or your CI's commit mechanism if you're not on GitHub.
resource "github_repository_file" "inventory" {
  repository          = "clusters"
  file                = "example.yaml"
  content             = module.inventory.rendered_yaml
  commit_message      = "chore(kcompass): update cluster inventory"
  overwrite_on_create = true
}

# kcompass_txt returns the TXT record value you'd publish at
# `kcompass.<your-zone>` for auto-discovery. It is provider-agnostic —
# feed the `record_value` output into a cloudflare_record, aws_route53_record,
# google_dns_record_set, or any other DNS resource.
module "txt" {
  source      = "../../modules/kcompass_txt"
  backend_url = "git@github.com:example-org/clusters"
}

output "inventory_yaml_preview" {
  description = "The YAML document that would be committed to the inventory repo."
  value       = module.inventory.rendered_yaml
}

output "txt_record_value" {
  value = module.txt.record_value
}

output "txt_record_name" {
  value = module.txt.record_name
}
