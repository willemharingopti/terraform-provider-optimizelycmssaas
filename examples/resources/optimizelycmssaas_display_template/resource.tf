resource "optimizelycmssaas_display_template" "article_layout" {
  key          = "ArticleLayout"
  display_name = "Article layout"
  content_type = optimizelycmssaas_content_type.article.key

  settings = {
    width = {
      display_name = "Width"
      editor       = "select"
      sort_order   = 10
      choices = {
        narrow = { display_name = "Narrow", sort_order = 10 }
        wide   = { display_name = "Wide", sort_order = 20 }
      }
    }
  }
}
