---
page_title: "Troubleshooting"
subcategory: ""
description: |-
  Common errors, what they mean, and how to fix them.
---

# Troubleshooting

## The CMS and read-after-write consistency

The API does not guarantee that a read made straight after a write sees it. Reads can return `404`, or the previous
version, for a few seconds, even after a read has succeeded once. The provider copes with this: after every create,
update, publish and delete it waits (at most ten seconds, best effort) until reads agree three times in a row, and it
uses the API's response to the write rather than reading again. You can see this as a fraction of a second added to each
write. It is also why a pipeline that reads an object immediately after Terraform finishes should allow a moment.

## Errors from the CMS

| Error | Meaning and what to do |
|---|---|
| `409 ... already exists` / `ConflictingResource` | The object is already there. [Import it](importing-existing-objects) or choose another key. |
| `403 ... do not have access rights to create ... of type 'X'` | The content type has `access_rights`, and your API client is not in one of the roles. Add the client to the role, or remove the restriction. |
| `403 Unable to access content item` | The API client cannot read that item. Applications and content can have their own access rules. |
| `400 ... considered breaking and could potentially result in data loss` | You are removing something that content may use (for example a property). Set `allow_data_loss = true` on the content type if that is what you intend. |
| `400 Could not read value as 'contentReference'` | `entry_point` must be `cms://content/<key>`. |
| `400 Application routing entry points cannot overlap each other` | Another application already points at this page, or at a page above or below it. |
| `400 A locale must be provided when creating content of a localized content type` | Set `locale` on the content resource. |
| `400 The value did not match the expected type` (content or blueprint) | Property values are wrapped: use `{ title = { value = "..." } }`, not `{ title = "..." }`. |
| `409 ... Unable to delete content type ... as content instances of this type exist` | Content of that type still exists, including soft-deleted content. Use `permanent_delete = true` on the content. |
| `400 Cannot delete content ... set as entry point for the '...' application` | An application still points at the page. Destroy the application first. The provider retries briefly, because the CMS can take a moment to notice a deleted application. |

## Errors from the provider or Terraform

**`client_id and client_secret are required`**
: The environment variables are not set in the shell that runs Terraform. Terraform does not read `.env` files; export the
  variables (see [Authentication and security](authentication)).

**`the local package ... doesn't match any of the checksums previously recorded in the dependency lock file`**
: You rebuilt a locally installed provider without changing its version. Run `rm -rf .terraform .terraform.lock.hcl`, then
  `terraform init`.

**`refusing request with an empty key`**
: Your state holds an entry with an empty key, typically left by an earlier failed apply. The provider refuses to send it,
  because an empty key would address the whole collection. Remove the entry with `terraform state rm <address>`; if the
  object exists on the instance, import it again.

**`Provider produced inconsistent result after apply`**
: This is a bug in the provider. Please report it with the resource type, your configuration and the error. The affected
  resource is marked tainted, so the next apply replaces it. Check the instance first, since the object may exist already
  (see the `409` above).

**`Content root not found`**
: On an instance whose root cannot be confirmed, and which has no readable application, the lookup has nothing to start from.
  Set `from_content` to the key of any content item the API client can read.

## Cleaning up after a failed run

A failed apply can leave objects behind on the instance that Terraform no longer tracks (for example after a state entry was
removed). Compare the instance with your configuration using the data sources, and either import what you want to keep
or delete the rest in the CMS. Be careful with content: soft-deleted items still count as instances of their type.

## Reporting a problem

When you report a problem, include the Terraform and provider versions, the resource involved, the (redacted) configuration,
and the full error, which carries the CMS's own message and a `traceId`. Remove any credentials first.
