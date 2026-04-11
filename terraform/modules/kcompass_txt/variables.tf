variable "backend_url" {
  description = "The kcompass backend URL to advertise in the TXT record — typically a git SSH or HTTPS URL pointing at the cluster inventory repository, or a local file path for air-gapped environments."
  type        = string

  validation {
    condition     = length(var.backend_url) > 0
    error_message = "backend_url must not be empty."
  }
}
