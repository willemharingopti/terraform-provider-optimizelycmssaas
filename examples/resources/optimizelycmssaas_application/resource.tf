resource "optimizelycmssaas_application" "site" {
  key          = "MainSite"
  display_name = "Main site"
  type         = "website"
  entry_point  = optimizelycmssaas_content.start.reference

  hosts = [
    { authority = "www.example.com", type = "primary", locale = "en", preferred_url_scheme = "https" },
  ]
}
