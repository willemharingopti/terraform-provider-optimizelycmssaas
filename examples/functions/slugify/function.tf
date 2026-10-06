locals {
  site_name = "Main site"
}

resource "optimizelycmssaas_application" "site" {
  key          = provider::optimizelycmssaas::slugify(local.site_name) # "main_site"
  display_name = local.site_name
  type         = "website"
  entry_point  = optimizelycmssaas_content.start.reference
}
