data "optimizelycmssaas_property_format" "short_string" {
  key = "shortString"
}

output "data_type" {
  value = data.optimizelycmssaas_property_format.short_string.data_type
}
