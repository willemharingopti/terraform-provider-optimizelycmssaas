# Finds the top of the content tree (the container that holds the sites' start pages).
data "optimizelycmssaas_content_root" "this" {}

resource "optimizelycmssaas_content" "site" {
  content_type = "StartPage"
  container    = data.optimizelycmssaas_content_root.this.key
  locale       = "en"
  display_name = "My site"
}

# Force the lookup to walk up from a specific application's start page:
data "optimizelycmssaas_content_root" "from_app" {
  from_application = "MainSite"
}
