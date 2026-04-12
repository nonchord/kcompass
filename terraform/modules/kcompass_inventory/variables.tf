variable "clusters" {
  description = <<-EOT
    Cluster records to render. Each record mirrors kcompass's ClusterRecord schema.
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
