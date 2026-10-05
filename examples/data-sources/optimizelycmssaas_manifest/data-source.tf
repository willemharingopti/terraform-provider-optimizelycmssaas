# Export the content model, e.g. to diff two environments.
data "optimizelycmssaas_manifest" "model" {
  sections          = ["contentTypes", "locales"]
  include_read_only = false
}

output "content_type_count" {
  value = length(jsondecode(data.optimizelycmssaas_manifest.model.json).contentTypes)
}
