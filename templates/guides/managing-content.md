---
page_title: "Managing content"
subcategory: ""
description: |-
  How the content resource works: versions, locales, publishing and deleting content items.
---

# Managing content

The [`optimizelycmssaas_content`](../resources/content) resource manages a content item together with **one locale's
current version**. It is best suited to structural content that belongs with your infrastructure, such as start pages,
settings and folders, rather than to day-to-day editorial content.

## A first item

```terraform
resource "optimizelycmssaas_content" "welcome" {
  content_type = "Article"
  container    = data.optimizelycmssaas_content_root.this.key # the parent item
  locale       = "en"                                         # required for localized types
  display_name = "Welcome"

  properties_json = jsonencode({
    heading = { value = "Hello" }
  })
}
```

- `container` is the key of the parent item. Changing it moves the item; changing `content_type`, `key`, `locale` or
  `blueprint` replaces it.
- `blueprint` creates the item from a blueprint. It is used only at creation, because the API does not return it.
- Property values are wrapped: `{ heading = { value = "Hello" } }`.

## Versions and publishing

Content has versions, with a status of `draft`, `ready`, `inReview`, `rejected`, `scheduled`, `published` or
`previouslyPublished`. The resource follows **one version**, shown in the read-only `version` and `status` attributes.

| You do | What happens |
|---|---|
| Create with `publish = false` (default) | A **draft** version is created. |
| Create, or later set, `publish = true` | The version is published at the end of the apply. |
| Change a **draft** | The draft is patched in place. |
| Change a **published** item | A **new draft version** is created with your changes (the published one stays live); set `publish = true` to publish it as well. |

`publish = true` is **one way**: setting it back to `false` does not unpublish. If the CMS accepts the publish request but
the version does not become published (for example because an approval workflow is required, or a publish is scheduled),
the apply fails with an error that says so instead of reporting a false success.

## Locales

Each resource covers **one locale** and creates the item. Adding further locales to an item that already exists is **not
supported yet**: a second resource with the same `key` would try to create the item again, which the CMS is expected to refuse with a conflict,
and destroying it would delete the whole item with all its locales. Manage additional locales in the CMS for now, and
keep one `optimizelycmssaas_content` resource per item.

## Experiences (composition)

An experience is a page whose body is a tree of sections, rows, columns and components. Set the tree with
`composition_json`; leave it unset and the composition is not managed. Drift is compared structurally, so fields the CMS
adds (such as node ids) are ignored. A change to the composition is always written as a new version of the page, because the CMS does not apply a patch to nodes it cannot match by id.

```hcl
resource "optimizelycmssaas_content" "home" {
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
```

The shape follows experiences read back from a real instance:

- The root is a node of type `experience` with `layoutType = "outline"`. Below it come `section`, `row`, `column` and
  `component` nodes. A component can also sit directly under the root.
- A section has `layoutType = "grid"` and a `component` of content type `BlankSection`.
- Property values inside `component.properties` are wrapped as `{ value = ... }`; rich text is `{ value = { html = "..." } }`.
- `displaySettings = { displayTemplate, settings }` is optional; every value in `settings` is a **string**, so write
  `"true"`, not `true`.
- The CMS adds an `id` to every node. You can leave `id` out; it does not cause drift.

## What is not tracked

- A newer version created in the CMS interface after Terraform's version is **not noticed**; the provider keeps following
  the version it created or imported.
- Additional locales of an existing item, media upload, copy, undelete and
  approvals are not supported.

## Deleting

By default, destroying a content item *soft-deletes* it: the CMS keeps it recoverable, and still counts it as an instance
of its content type, so the content type cannot be deleted in the same run (*409, content instances exist*).
Set `permanent_delete = true` to remove it for good on destroy. Choose deliberately: permanent deletion cannot be undone.

Deleting a page that an application uses as its entry point is refused by the CMS. Destroy the application first
(Terraform does this automatically when the application references the page).
