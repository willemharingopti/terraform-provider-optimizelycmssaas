---
page_title: "Authentication and security"
subcategory: ""
description: |-
  How the provider authenticates, how to keep the client secret out of your state, and what is stored where.
---

# Authentication and security

## How authentication works

The provider uses the OAuth 2.0 *client credentials* grant against `/oauth/token` with your API client's name and
secret, and sends the resulting bearer token with every request. Tokens are short-lived (about five minutes). The provider
caches the token in memory only, refreshes it before it expires, and requests a new one once if the API answers `401`. A token is never written
to disk, to the state file or to logs.

## Configuration options

| Setting | Environment variable | Default |
|---|---|---|
| `client_id` | `OPTIMIZELY_CMS_CLIENT_ID` | none (required) |
| `client_secret` | `OPTIMIZELY_CMS_CLIENT_SECRET` | none (required) |
| `base_url` | `OPTIMIZELY_CMS_BASE_URL` | `https://api.cms.optimizely.com` |
| `api_version` | `OPTIMIZELY_CMS_API_VERSION` | `v1` |
| `content_root_key` | `OPTIMIZELY_CMS_CONTENT_ROOT_KEY` | `43f936c99b234ea397b261c538ad07c9` |
| `token_url` | `OPTIMIZELY_CMS_TOKEN_URL` | `<base_url>/oauth/token` |

A setting in the `provider` block wins over the environment variable. `base_url` is the *host*: the provider adds the
API version to it, so do not include one.

## Choosing an API version

Requests go to `<base_url>/<api_version>/...`, for example `https://api.cms.optimizely.com/v1/locales`. The version
is resolved in this order:

1. `api_version` in the `provider` block, or `OPTIMIZELY_CMS_API_VERSION`;
2. otherwise a version at the end of `base_url` (`https://api.cms.optimizely.com/v2` selects `v2`);
3. otherwise `v1`.

The value must have the form `v1`, `v2`, ... Anything else is rejected when the provider is configured, because the version
becomes part of every request path.

~> **The provider is built and tested against `v1`** (OpenAPI specification 1.1). A newer version may rename or restructure
resources, so using one is at your own risk until the provider is updated for it. At the time of writing, the API
answers `404` for `/v2`.

## Choosing the content root

`content_root_key` is the key of the top of the content tree, used by the
[`optimizelycmssaas_content_root`](../data-sources/content_root) data source. The default is the built-in root observed on every
instance checked, which is not documented by Optimizely, so it can be overridden if your instance uses another one:

```shell
export OPTIMIZELY_CMS_CONTENT_ROOT_KEY="43f936c99b234ea397b261c538ad07c9"
```

A key written with dashes (`43f936c9-9b23-4ea3-97b2-61c538ad07c9`) is accepted and converted, because the API itself only accepts
the form without dashes. If you **set** the key, it is used or the lookup fails with an error that names it; the provider
never silently falls back to a different root. If you leave it unset, the default is tried first and, when it is not
present, the root is found from an existing application's start page.

## Keeping the secret safe

- **Prefer environment variables.** If you write `client_secret` in the `provider` block, it ends up in your
  configuration files and in any saved plan file (`terraform plan -out`), which includes the provider configuration. The
  attribute is marked sensitive, which hides it from console output but does not remove it from those files.
- **Use a secrets manager** (CI secret store, a vault) to inject the variables at run time. Never commit credentials,
  and add `.env` to `.gitignore`.
- **Use one API client per environment** and give it only the access it needs. A client that cannot create
  content of a given type will receive `403` for it; see [Troubleshooting](troubleshooting).
- **Rotate the secret** if it may have been exposed, for example if a terminal session or log containing it was shared.

## Transport

Only `https` URLs are accepted for `base_url` and `token_url`; an `http` URL is rejected when the provider is
configured. Error messages from failed token requests deliberately do not include the response body, because it could
echo credentials.

## What ends up in your state

Terraform state contains the attributes of every managed object, including content property values you supply
(`properties_json`, `content_json`). The provider does not mark these sensitive, so **do not put secrets in content
properties**, and store state in a backend with access control and encryption.

## Request behaviour

- Requests are retried when the API answers `429 Too Many Requests`, honouring `Retry-After` (up to three retries).
- Responses are size-limited, and requests time out after 30 seconds.
- Keys are URL-escaped before they are used in a path, and an empty key is refused instead of being sent.
