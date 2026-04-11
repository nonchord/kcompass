# kcompass_txt

A pure value-formatter module for kcompass's auto-discovery TXT record. Takes a backend URL, returns the properly formatted `v=kc1; backend=<url>` TXT record value plus the conventional DNS label (`kcompass`). No resources, no providers — feed the outputs into whichever DNS provider you use (Cloudflare, Route53, Google DNS, etc.).

The value-in, value-out shape means that if kcompass's discovery format evolves (e.g. a future `v=kc2` with additional fields), callers bump the module version and the change ripples through without touching their DNS provider wiring.

## Usage

### With Cloudflare

```hcl
module "kcompass_txt" {
  source      = "git::https://github.com/nonchord/kcompass.git//terraform/modules/kcompass_txt?ref=<tag>"
  backend_url = "git@github.com:example-org/clusters"
}

data "cloudflare_zone" "this" {
  name = "example-org.com"
}

resource "cloudflare_record" "kcompass" {
  zone_id = data.cloudflare_zone.this.id
  name    = module.kcompass_txt.record_name
  type    = "TXT"
  value   = module.kcompass_txt.record_value
  ttl     = 300
}
```

### With AWS Route 53

```hcl
resource "aws_route53_record" "kcompass" {
  zone_id = var.zone_id
  name    = "${module.kcompass_txt.record_name}.${var.zone_name}"
  type    = "TXT"
  ttl     = 300
  records = [module.kcompass_txt.record_value]
}
```

### With Google Cloud DNS

```hcl
resource "google_dns_record_set" "kcompass" {
  managed_zone = var.managed_zone
  name         = "${module.kcompass_txt.record_name}.${var.zone_name}."
  type         = "TXT"
  ttl          = 300
  rrdatas      = ["\"${module.kcompass_txt.record_value}\""]
}
```

## Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `backend_url` | string | — | The kcompass backend URL to advertise. Typically a git SSH/HTTPS URL, or a local file path for air-gapped environments. |

## Outputs

| Name | Description |
|---|---|
| `record_value` | The TXT record value to publish: `v=kc1; backend=<backend_url>`. |
| `record_name` | The suggested DNS label for the record (`kcompass`). kcompass probes `<record_name>.<zone>` for discovery. |
