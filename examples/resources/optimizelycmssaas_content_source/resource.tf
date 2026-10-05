# Content sources surface items from an external source (for example Optimizely Graph).
# The CMS ships built-in ones (Item, AssetItem, ...); this manages your own.
resource "optimizelycmssaas_content_source" "products" {
  key          = "Products"
  type         = "graph"
  source_key   = "default"
  source_type  = "Product"
  display_name = "Products"
  base_type    = "_page"

  property_mappings = {
    key          = "_itemMetadata.key"
    display_name = "_itemMetadata.displayName"
  }
}
