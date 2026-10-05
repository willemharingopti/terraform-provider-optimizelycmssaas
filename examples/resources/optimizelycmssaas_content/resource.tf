data "optimizelycmssaas_content_root" "this" {}

resource "optimizelycmssaas_content" "start" {
  content_type = optimizelycmssaas_content_type.article.key
  container    = data.optimizelycmssaas_content_root.this.key
  locale       = "en"
  display_name = "Welcome"

  properties_json = jsonencode({
    heading = { value = "Hello" }
  })

  publish          = false # set true to publish after each apply
  permanent_delete = true  # remove for good on destroy
}
