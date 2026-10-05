resource "optimizelycmssaas_content_type_binding" "product_to_page" {
  key  = "ProductToPage"
  from = "Product"
  to   = optimizelycmssaas_content_type.article.key

  # Map key = the path the property binds to.
  property_mappings = {
    heading = { from = "name" }
  }
}
