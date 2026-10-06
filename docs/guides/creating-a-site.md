---
page_title: "Creating a site"
subcategory: ""
description: |-
  Create a new site: a start page under the content root, and an application that points at it.
---

# Creating a site

A *site* in Optimizely CMS (SaaS) is an **application** whose *entry point* is a **start page** in the content tree. The
content tree has a single top-level container, the *content root*, which holds the sites' start pages:

```text
root (top-level container)
 +- <your site>          the start page: a content item of a page type
     +- ...              its pages
```

So creating a site takes three steps: find the root, create the start page below it, and create the application that
points at the start page. The complete working example is in
`examples/complete` in the provider's repository.

## 1. Find the content root

```terraform
data "optimizelycmssaas_content_root" "this" {}
```

The API has no endpoint that returns the root, so the data source finds it: it first checks the root's key (by default the
fixed `43f936c99b234ea397b261c538ad07c9`, observed to be identical on separate instances and verified before use; override
it with `content_root_key` or `OPTIMIZELY_CMS_CONTENT_ROOT_KEY`), and otherwise walks up from the start page of an existing
application that the API client may read. This means it also works
on a brand-new, empty instance. To force the walk-up from a particular place, set `from_application` or `from_content`.

## 2. A page type and the start page

A new instance has no page types, so define one (skip this if you already have a suitable type):

```terraform
resource "optimizelycmssaas_content_type" "start_page" {
  key          = "StartPage"
  base_type    = "_page"
  display_name = "Start page"

  properties_json = jsonencode({
    heading = { type = "string", displayName = "Heading" }
  })
}

resource "optimizelycmssaas_content" "site" {
  content_type = optimizelycmssaas_content_type.start_page.key
  container    = data.optimizelycmssaas_content_root.this.key
  locale       = "en" # required: page types are localized
  display_name = var.site_name

  publish          = false # created as a draft; set true to publish
  permanent_delete = true  # see "Destroying a site" below
}
```

The `locale` must already exist on the instance (see [`optimizelycmssaas_locale`](../resources/locale)).

## 3. The application

```terraform
resource "optimizelycmssaas_application" "site" {
  key          = replace(var.site_name, "/[^0-9A-Za-z_]/", "") # letters, digits, underscore; starts with a letter
  display_name = var.site_name
  type         = "website"
  entry_point  = optimizelycmssaas_content.site.reference

  hosts = [
    { authority = "www.example.com", type = "primary", locale = "en", preferred_url_scheme = "https" },
  ]
}
```

Things to know:

- The `entry_point` must be a content reference: `cms://content/<key>`. A plain name such as `mysite` is rejected.
  If you add `?loc=en`, the CMS stores the reference without it; the provider accounts for that and reports no
  difference.
- **Entry points cannot overlap.** Two applications cannot point at the same page, or at a page and one of its
  descendants.
- `hosts` is optional; omit it to create the application without hostnames.

## Destroying a site

Deleting a content item normally *soft-deletes* it: the CMS keeps it in a recoverable state, and still counts it as an
instance of its content type. Destroying the content type in the same run then fails with
*409 "Unable to delete content type ... as content instances of this type exist"*. Setting `permanent_delete = true` on
the start page, as above, removes it for good and avoids this. Terraform destroys the application first, then the page,
then its type.

## A reusable pattern

Because the key and the page both derive from one name, you can wrap the three steps in a module that takes a
`site_name` variable and outputs the application key and the start page key.
