terraform {
  required_version = ">= 1.3.0"

  required_providers {
    github = {
      source  = "integrations/github"
      version = ">= 6.0"
    }
  }
}

locals {
  inventory = {
    clusters = [
      for c in var.clusters : merge(
        { name = c.name },
        c.description == "" ? {} : { description = c.description },
        length(c.labels) == 0 ? {} : { labels = c.labels },
        {
          kubeconfig = merge(
            c.kubeconfig.inline != null ? { inline = c.kubeconfig.inline } : {},
            c.kubeconfig.command != null ? { command = c.kubeconfig.command } : {},
          )
        },
      )
    ]
  }

  rendered = "# Managed by terraform module kcompass_inventory. Do not edit by hand.\n${yamlencode(local.inventory)}"
}

resource "github_repository_file" "inventory" {
  repository          = var.repository
  file                = var.filename
  content             = local.rendered
  branch              = var.branch
  commit_message      = var.commit_message
  overwrite_on_create = true
}
