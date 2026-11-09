# Changelog

All notable changes are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/), and the project
uses [Semantic Versioning](https://semver.org/). While the version is below 1.0, minor releases may contain breaking changes.

## [Unreleased]

## [0.2.0]

### Added
- `optimizelycmssaas_content` accepts `composition_json` to manage the section/row/column/component tree of an experience.
  Leave it unset to keep not managing composition. Drift is compared structurally, so ids and defaults the CMS adds are
  ignored, and a configuration without them plans as no change after an import.

### Notes
- A change to the composition is written as a new version of the page (drafts included), because the CMS does not apply
  a version patch to nodes it cannot match by id. Each such change adds a version to the page's history.
- `properties_json` and `composition_json` handle drift the same way, except that only `composition_json` keeps the
  recorded value at plan time when the configuration is a subset of it.

## [0.1.1]

### Added
- `optimizelycmssaas_content` exports a computed `reference` (`cms://content/<key>`), for use as an application's `entry_point`.
- Provider function `slugify`, e.g. `provider::optimizelycmssaas::slugify("Main site")` is `main_site`. Needs Terraform 1.8 or later.

## [0.1.0]

First release.

### Added
- Resources: `optimizelycmssaas_content_type`, `_content_source`, `_content_type_binding`, `_locale`, `_display_template`,
  `_application`, `_property_group`, `_blueprint` and `_content`.
- Data sources: a by-key lookup for each resource except `content`, plus `_property_format`, `_manifest` and `_content_root`.
- Provider settings `api_version` and `content_root_key` (and their environment variables).
- Registry documentation: reference pages, examples and seven guides.

### Notes
- Built and tested against the Optimizely CMS (SaaS) REST API `v1` (OpenAPI specification 1.1).
- Not supported: manifest import, content composition (added in 0.2.0), media upload, adding locales to an existing content item, and approval
  workflows. See the README.
