resource "optimizelycmssaas_blueprint" "article_starter" {
  display_name = "Article with placeholder heading"
  content_type = optimizelycmssaas_content_type.article.key

  # Every property value is wrapped as { value = ... }.
  content_json = jsonencode({
    properties = {
      heading = { value = "Your heading here" }
    }
  })
}
