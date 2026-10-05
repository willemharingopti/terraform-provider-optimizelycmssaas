resource "optimizelycmssaas_locale" "swedish" {
  key           = "sv" # IETF BCP-47 tag
  display_name  = "Swedish"
  route_segment = "sv"
  fallback      = "en"
}
