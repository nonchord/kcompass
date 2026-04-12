terraform {
  required_version = ">= 1.3.0"
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
}
