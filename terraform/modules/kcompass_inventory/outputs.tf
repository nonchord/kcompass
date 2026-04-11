output "filename" {
  description = "The file path written inside the inventory repository."
  value       = github_repository_file.inventory.file
}

output "rendered_yaml" {
  description = "The full YAML document committed to the repository. Useful for local preview or tests."
  value       = local.rendered
}
