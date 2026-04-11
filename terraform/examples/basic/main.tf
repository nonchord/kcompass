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

# kcompass_inventory writes one YAML file into an existing GitHub repo.
# The repo itself is expected to already exist — create it with the
# integrations/github provider's github_repository resource outside this
# module (and optionally seed a README via auto_init = true).
module "inventory" {
  source = "../../modules/kcompass_inventory"

  repository = "clusters"
  filename   = "example.yaml"

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
