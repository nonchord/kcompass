output "rendered_yaml" {
  description = "The rendered kcompass inventory YAML document. Feed this into a git provider resource, a local_file, or any other mechanism that commits it to your cluster inventory repository."
  value       = "# Managed by terraform module kcompass_inventory. Do not edit by hand.\n${yamlencode(local.inventory)}"
}
