terraform {
  required_providers {
    optimizelycmssaas = {
      source = "willemharingopti/optimizelycmssaas" # placeholder address, see README
    }
  }
}

# Credentials come from the environment, keeping the secret out of config and state:
#   OPTIMIZELY_CMS_CLIENT_ID, OPTIMIZELY_CMS_CLIENT_SECRET
# Optional: OPTIMIZELY_CMS_BASE_URL (host only, default https://api.cms.optimizely.com)
provider "optimizelycmssaas" {}

variable "site_name" {
  description = "Name of the new site. Becomes the application's display name and the start page's title."
  type        = string

  validation {
    condition     = length(trimspace(var.site_name)) > 0 && can(regex("^[A-Za-z][_0-9A-Za-z]+$", replace(var.site_name, "/[^0-9A-Za-z_]/", "")))
    error_message = "site_name must contain at least two letters/digits and start with a letter, e.g. \"My Brand\"."
  }
}

variable "start_page_locale" {
  description = "Locale of the start page. Must already exist."
  type        = string
  default     = "en"
}

variable "host" {
  description = "Optional primary host name for the site, e.g. www.example.com."
  type        = string
  default     = null
}

locals {
  # "My Brand" -> "MyBrand": a valid application key (letters, digits, underscore).
  site_key = replace(var.site_name, "/[^0-9A-Za-z_]/", "")
}

# --- Content model ---------------------------------------------------------
# WARNING: `terraform apply` creates real objects. Every key below (Example*, locale
# "fo", and the key derived from site_name) must be free on the target instance, or the CMS answers 409 Conflict. To manage
# something that already exists, `terraform import` it instead.

resource "optimizelycmssaas_locale" "faroese" {
  key           = "fo"
  display_name  = "Faroese"
  route_segment = "fo"
  fallback      = "en"
}

resource "optimizelycmssaas_property_group" "seo" {
  key          = "ExampleSeoGroup"
  display_name = "SEO"
  sort_order   = 15
}

resource "optimizelycmssaas_content_type" "teaser" {
  key          = "ExampleTeaser"
  base_type    = "_component"
  display_name = "Example teaser"

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

  # access_rights restricts WHO MAY CREATE content of this type, e.g.
  #   access_rights = [{ name = "WebAdmins" }]   # type defaults to "role"
  # Leave it out unless your API client belongs to that role, or it will be refused
  # (403) when creating content or blueprints of the type.
}

resource "optimizelycmssaas_display_template" "teaser_width" {
  key          = "ExampleTeaserDefault"
  display_name = "Teaser default"
  content_type = optimizelycmssaas_content_type.teaser.key
  is_default   = true

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

resource "optimizelycmssaas_blueprint" "teaser" {
  display_name = "Teaser with placeholder text"
  content_type = optimizelycmssaas_content_type.teaser.key
  content_json = jsonencode({ properties = { heading = { value = "Your heading here" } } })
}

# --- Site ------------------------------------------------------------------
# root (top-level container)
#   +- <site_name>          the site's start page, created below (the "site")
# The application's entry point references that start page.

# The root of the content tree, found by walking up from an existing site's start page.
data "optimizelycmssaas_content_root" "this" {}

# A minimal page type for the site's start page (new instances have no page types).
resource "optimizelycmssaas_content_type" "start_page" {
  key          = "ExampleStartPage"
  base_type    = "_page"
  display_name = "Example start page"

  properties_json = jsonencode({
    heading = { type = "string", displayName = "Heading" }
  })
}

resource "optimizelycmssaas_content" "site" {
  content_type = optimizelycmssaas_content_type.start_page.key
  container    = data.optimizelycmssaas_content_root.this.key
  locale       = var.start_page_locale
  display_name = var.site_name
  # Created as a draft. Set publish = true to make the site live.
  publish = false

  # Delete permanently on destroy. A soft-deleted page still counts as an instance of its
  # content type, so destroying the type afterwards would fail with 409 "content instances exist".
  permanent_delete = true
}

resource "optimizelycmssaas_application" "site" {
  key          = local.site_key
  display_name = var.site_name
  type         = "website"
  entry_point  = "cms://content/${optimizelycmssaas_content.site.key}"

  hosts = var.host == null ? null : [
    { authority = var.host, type = "primary", locale = var.start_page_locale, preferred_url_scheme = "https" },
  ]
}

# --- Lookups (read-only) ---------------------------------------------------

data "optimizelycmssaas_property_format" "short" {
  key = "shortString"
}

data "optimizelycmssaas_manifest" "model" {
  sections = ["contentTypes", "locales"]
}

data "optimizelycmssaas_locale" "english" {
  key = "en"
}

output "english_route_segment" {
  value = data.optimizelycmssaas_locale.english.route_segment
}

output "site" {
  value = {
    application = optimizelycmssaas_application.site.key
    start_page  = optimizelycmssaas_content.site.key
    root        = data.optimizelycmssaas_content_root.this.key
  }
}
