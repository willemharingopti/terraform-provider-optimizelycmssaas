resource "optimizelycmssaas_content_type" "article" {
  key          = "Article"
  base_type    = "_page"
  display_name = "Article"

  may_contain_types = ["Article", "_component"]

  # Property definitions, keyed by property name. Use jsonencode().
  properties_json = jsonencode({
    heading = {
      type        = "string"
      displayName = "Heading"
      isRequired  = true
      maxLength   = 120
    }
    body = {
      type        = "richText"
      displayName = "Body"
      group       = optimizelycmssaas_property_group.seo.key
    }
  })
}

# Removing a property deletes its stored values, so the CMS refuses it unless you opt in:
#   allow_data_loss = true
