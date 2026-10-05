data "optimizelycmssaas_application" "site" {
  key = "MainSite"
}

output "entry_point" {
  value = data.optimizelycmssaas_application.site.entry_point
}
