resource "optimizelycmssaas_application" "site" {
  key          = "MainSite"
  display_name = "Main site"
  type         = "website"
  entry_point  = "cms://content/${optimizelycmssaas_content.start.key}"

  hosts = [
    { authority = "www.example.com", type = "primary", locale = "en", preferred_url_scheme = "https" },
  ]
}
