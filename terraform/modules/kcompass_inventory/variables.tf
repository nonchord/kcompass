variable "repository" {
  description = "Name of the existing GitHub repository (under the configured provider owner) where the inventory file is written, e.g. \"clusters\". The repo must exist — this module does not create it."
  type        = string
}

variable "filename" {
  description = "Path within the repository where this inventory file is written, e.g. \"staging.yaml\". Multiple callers can write to the same repo as long as each uses a distinct filename; kcompass's git backend scans every *.yaml file and merges them."
  type        = string
}

variable "branch" {
  description = "Branch to commit the inventory file on. Defaults to the repository's default branch."
  type        = string
  default     = null
}

variable "commit_message" {
  description = "Commit message for create/update operations on the inventory file."
  type        = string
  default     = "chore(kcompass): update cluster inventory"
}

variable "clusters" {
  description = <<-EOT
    Cluster records to publish. Each record mirrors kcompass's ClusterRecord schema.
    Exactly one of `kubeconfig.inline` or `kubeconfig.command` must be set per record:

      - `inline`: a complete kubeconfig YAML document shipped with the record. Use
        when the same kubeconfig works for every consumer (e.g. an OIDC exec plugin,
        a service account token behind a VPN).
      - `command`: an argv vector kcompass runs at connect time to mint a per-user
        kubeconfig. The command must honor KUBECONFIG and write to the file it points at.
        Examples: `[tailscale, configure, kubeconfig, my-cluster]`,
        `[gcloud, container, clusters, get-credentials, prod, --region, us-central1]`,
        `[aws, eks, update-kubeconfig, --name, prod, --region, us-west-2]`.
  EOT

  type = list(object({
    name        = string
    description = optional(string, "")
    labels      = optional(map(string), {})
    kubeconfig = object({
      inline  = optional(string)
      command = optional(list(string))
    })
  }))

  validation {
    condition = alltrue([
      for c in var.clusters :
      (c.kubeconfig.inline != null) != (c.kubeconfig.command != null)
    ])
    error_message = "Each cluster must set exactly one of kubeconfig.inline or kubeconfig.command."
  }
}
