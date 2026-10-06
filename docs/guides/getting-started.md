---
page_title: "Getting started"
subcategory: ""
description: |-
  Install the provider, authenticate against your Optimizely CMS (SaaS) instance, and run a first read-only configuration.
---

# Getting started

This provider manages the content model and configuration of an Optimizely CMS (SaaS) instance through its
[REST API](https://docs.optimizely.com/cms-saas/api/optimizely-cms-rest-api-content): content types, locales,
display templates, blueprints, applications and content items.

## Before you start

You need:

- An Optimizely CMS (SaaS) instance. **Use a sandbox first**: applying a configuration creates, changes and
  deletes real objects.
- An **API client** for that instance, with its *client name* and *client secret*. See
  [Request access token](https://docs.optimizely.com/cms-saas/api/request-access-token) in the Optimizely documentation.
  Note that the provider's `client_id` is the API client's **name**.
- Terraform 1.0 or later.

## Install

Declare the provider in your configuration:

```terraform
terraform {
  required_providers {
    optimizelycmssaas = {
      source = "willemharingopti/optimizelycmssaas" # replace with the address you publish under
    }
  }
}
```

Building from source and installing it locally instead:

```shell
go build -o terraform-provider-optimizelycmssaas .
mkdir -p ~/.terraform.d/plugins/registry.terraform.io/willemharingopti/optimizelycmssaas/0.0.1/darwin_arm64
cp terraform-provider-optimizelycmssaas \
   ~/.terraform.d/plugins/registry.terraform.io/willemharingopti/optimizelycmssaas/0.0.1/darwin_arm64/terraform-provider-optimizelycmssaas_v0.0.1
terraform init
```

Adjust the version and the `darwin_arm64` platform folder to match your build. If you rebuild the binary under the
same version, delete `.terraform.lock.hcl` and `.terraform/` before running `terraform init` again, otherwise Terraform
rejects the new binary because its checksum no longer matches the lock file.

## Authenticate

Export the credentials; the provider reads them from the environment:

```shell
export OPTIMIZELY_CMS_CLIENT_ID="my-api-client"
export OPTIMIZELY_CMS_CLIENT_SECRET="..."
export OPTIMIZELY_CMS_BASE_URL="https://api.cms.optimizely.com"   # optional; this is the default
```

~> **Terraform does not read `.env` files.** If you keep the variables in one, load it into your shell first, for
example `set -a; source .env; set +a`.

See [Authentication and security](authentication) for details and recommendations.

## A first, read-only configuration

This reads data and changes nothing, so it is a safe way to check that credentials and connectivity work:

```terraform
provider "optimizelycmssaas" {}

data "optimizelycmssaas_locale" "english" {
  key = "en"
}

data "optimizelycmssaas_content_root" "this" {}

output "english_route_segment" {
  value = data.optimizelycmssaas_locale.english.route_segment
}

output "content_root" {
  value = data.optimizelycmssaas_content_root.this.key
}
```

```shell
terraform init
terraform apply
```

If you see `client_id and client_secret are required`, the environment variables are not set in the shell you are running
Terraform from.

## Next steps

- [Creating a site](creating-a-site): the start page, the application and the content root.
- [Modelling content](modelling-content): content types, properties, display templates and blueprints.
- [Managing content](managing-content): content items, versions and publishing.
- [Importing existing objects](importing-existing-objects): adopt what is already on your instance.
- [Troubleshooting](troubleshooting): the errors you are most likely to meet, and what they mean.
