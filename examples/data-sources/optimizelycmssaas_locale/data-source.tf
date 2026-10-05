data "optimizelycmssaas_locale" "english" {
  key = "en"
}

output "english_route" {
  value = data.optimizelycmssaas_locale.english.route_segment
}
