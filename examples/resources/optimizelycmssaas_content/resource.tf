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

# An experience: a page whose body is a tree of sections, rows, columns and components.
resource "optimizelycmssaas_content" "experience" {
  content_type = "BlankExperience"
  container    = data.optimizelycmssaas_content_root.this.key
  locale       = "en"
  display_name = "Home"

  composition_json = jsonencode({
    nodeType   = "experience"
    layoutType = "outline"
    nodes = [{
      nodeType    = "section"
      displayName = "Intro"
      layoutType  = "grid"
      component   = { contentType = "BlankSection" }
      nodes = [{
        nodeType = "row"
        nodes = [{
          nodeType = "column"
          nodes = [{
            nodeType    = "component"
            displayName = "Hello"
            component = {
              contentType = "HeroVideo"
              properties  = { heading = { value = "Hello" } }
            }
          }]
        }]
      }]
    }]
  })
}
