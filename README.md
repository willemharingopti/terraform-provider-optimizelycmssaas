# Terraform provider for Optimizely CMS (SaaS)

Manages the content model and configuration of an Optimizely CMS (SaaS) instance
through its [REST API (v1)](https://docs.optimizely.com/cms-saas/api/optimizely-cms-rest-api-content),
built on the Terraform Plugin Framework.

> **Status: early.** Built against the API's OpenAPI spec (v1.1). Reads are verified against a live instance; write paths
> are covered by unit tests with a mock API and by acceptance tests (see *Testing*), which you should run against a sandbox
> before relying on this. The module path and registry address are placeholders (`example`).

## Documentation

Reference pages for every resource and data source, and step-by-step guides, are in [docs/](docs/):

| Guide | Covers |
|---|---|
| [Getting started](docs/guides/getting-started.md) | install, credentials, a first read-only configuration |
| [Authentication and security](docs/guides/authentication.md) | how auth works, keeping the secret out of state |
| [Creating a site](docs/guides/creating-a-site.md) | content root, start page, application, destroying |
| [Modelling content](docs/guides/modelling-content.md) | content types, properties, display templates, blueprints |
| [Managing content](docs/guides/managing-content.md) | versions, publishing, deleting |
| [Importing existing objects](docs/guides/importing-existing-objects.md) | `terraform import` and drift |
| [Troubleshooting](docs/guides/troubleshooting.md) | common errors and fixes |

The reference pages under `docs/resources/` and `docs/data-sources/` are **generated** from the provider's schema and
`examples/`; the guides live in `templates/guides/` and are copied to `docs/guides/`. Regenerate after changing a schema
description, an example or a guide (needs `terraform` on your `PATH`):

```sh
go generate ./...        # runs tfplugindocs; see the directive in main.go
```

## Resources and data sources

| Resource | Notes |
|---|---|
| `optimizelycmssaas_content_type` | Properties as `properties_json` (use `jsonencode`); drift detected structurally. |
| `optimizelycmssaas_content_source` | External sources (e.g. Graph). `property_mappings.key` is create-only. |
| `optimizelycmssaas_content_type_binding` | Maps one content type onto another. |
| `optimizelycmssaas_locale` | Key is a BCP-47 tag. |
| `optimizelycmssaas_display_template` | Typed `settings` → `choices`. |
| `optimizelycmssaas_application` | Websites and their hosts. |
| `optimizelycmssaas_property_group` | |
| `optimizelycmssaas_blueprint` | Template property values as `content_json`. |
| `optimizelycmssaas_content` | A content item plus **one locale's** current version; optional `publish`. |

Data sources: one by-key lookup for every resource above except `content`, plus `optimizelycmssaas_property_format`
(read-only in the API), `optimizelycmssaas_manifest` (content-model export as JSON) and `optimizelycmssaas_content_root`.

`optimizelycmssaas_content_root` finds the top of the content tree (the container that holds the sites' start pages). The
API has no root endpoint, so it first checks the built-in root's fixed key (`43f936c99b234ea397b261c538ad07c9`, observed identical
on separate instances but not documented, so it is verified before use), and otherwise walks up from the start page of the first
existing application the API client can read. Set `from_application` / `from_content` to force the walk-up. Use its `key` as the `container` of a
new site's start page; see the *Site* section of the example.

See [examples/complete/main.tf](examples/complete/main.tf).

All resources support `terraform import` by key. `optimizelycmssaas_content` accepts `key` or `key/locale`.

## Authentication

Create an API client in the CMS, then export its credentials. `client_id` is the API client's **name**.

```sh
export OPTIMIZELY_CMS_CLIENT_ID=...
export OPTIMIZELY_CMS_CLIENT_SECRET=...
export OPTIMIZELY_CMS_BASE_URL=https://api.cms.optimizely.com   # optional; host only, the version is added
export OPTIMIZELY_CMS_API_VERSION=v1                             # optional; default v1 (the version this provider targets)
export OPTIMIZELY_CMS_CONTENT_ROOT_KEY=43f936c99b234ea397b261c538ad07c9   # optional; the content tree root (this is the default)
```

Prefer environment variables over the `client_secret` provider argument so the secret stays out of config and state.
Only `https` URLs are accepted. The client obtains short-lived tokens (about 5 minutes) with the OAuth client-credentials
grant, refreshes them as needed, and retries on HTTP 429.

## Behaviour worth knowing

- **Merge-patch updates.** Items you remove from config (a property, a setting, a choice, a mapping) are deleted by sending
  `null`. Fields the API cannot change (`key`, `base_type`, content `locale`, …) force a replacement.
- **`properties_json` / `content_json` drift.** The configured JSON is kept as long as it is a structural subset of what the
  API returns, so server-added defaults do not cause diffs. A property added out of band to a *content type* is detected;
  extra top-level keys in blueprint/content JSON are not.
- **Content versions.** `optimizelycmssaas_content` tracks the version it created or imported. Editing a *published* version
  creates a new draft; a draft is patched in place. `publish = true` publishes after each apply and is one-way
  (it never unpublishes). A newer version created in the UI is not noticed.
- **Property values are wrapped.** In `properties_json` (content) and `content_json` (blueprint) every value is an object
  with a `value` field, e.g. `jsonencode({ properties = { title = { value = "Hello" } } })`. A bare string is rejected by the API.
- **Breaking content type changes.** Removing a property deletes its stored values, so the CMS refuses it with a data-loss
  error. Set `allow_data_loss = true` on the content type to let that update through.
- **Display template defaults.** The CMS makes the first template for a type the default whatever `is_default` says;
  leave `is_default` unset unless you need to control it.
- **Eventual consistency.** The API does not guarantee that a read straight after a write sees it: reads can 404 or return
  the previous version for a few seconds, even after one read has succeeded. After every create, update, publish and delete the
  provider therefore waits (up to 10 s, best effort) until reads agree three times in a row, and uses the API's response
  (`Prefer: return=representation`) rather than re-reading. A page that is still an application's entry point right after
  that application was deleted is retried for the same window.
- **Deleting content.** By default `terraform destroy` only soft-deletes a content item, and the CMS still counts it as an
  instance of its type, so destroying that content type in the same run fails with *409 content instances of this type exist*.
  Set `permanent_delete = true` on content you create with Terraform (the example does) if you want it removed for good.
- **Built-in objects** (e.g. system content types and property groups) cannot be changed or deleted by the API.

## Not supported

Manifest **import** (bulk, partly irreversible), content composition (experiences/sections), media upload, content
copy/undelete, approval workflows, and the `Prefer`, `cms-skip-validation` and `If-Match` headers.

## Development

```sh
go build ./... && go vet ./... && go test ./...      # unit tests; live read-only checks run if credentials are set
```

To try a local build without publishing, build the binary and load it through `dev_overrides`.
[examples/complete/dev.tfrc](examples/complete/dev.tfrc) already does this with a relative path:

```sh
go build -o terraform-provider-optimizelycmssaas .          # repo root
cd examples/complete
export TF_CLI_CONFIG_FILE=$PWD/dev.tfrc
terraform validate                                      # no `terraform init` needed with dev_overrides
terraform plan -var entry_point=cms://content/<id>
```

### Testing

- **Unit tests** use a mock HTTPS server and need nothing external.
- **`TestLiveReadOnly`** performs GET requests only; it runs when `OPTIMIZELY_CMS_CLIENT_ID/SECRET` are set.
- **Acceptance tests** (`TestAcc*`) drive a real Terraform binary against a real instance and **create and delete objects**
  prefixed `TfAcc` (and the locale `gd`). Use a sandbox.

  ```sh
  TF_ACC=1 TF_ACC_TERRAFORM_PATH=$(which terraform) go test ./internal/provider -run TestAcc -v -timeout 30m
  ```

  `TestAccApplication` and `TestAccContent` need `OPTIMIZELY_CMS_TEST_CONTAINER` (e.g. the content root key) and
  `OPTIMIZELY_CMS_TEST_CONTENT_TYPE` (a localized page type allowed there). They skip when unset.
  `TestAccReadOnlyDataSources` only reads and is safe anywhere.
