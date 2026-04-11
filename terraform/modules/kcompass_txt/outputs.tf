output "record_value" {
  description = "The TXT record value to publish. Feed this to your DNS provider resource's `value`/`content` field."
  value       = "v=kc1; backend=${var.backend_url}"
}

output "record_name" {
  description = "The suggested DNS label for the record. kcompass probes `<record_name>.<zone>` — defaulting to `kcompass` here keeps operators aligned with the probe convention."
  value       = "kcompass"
}
