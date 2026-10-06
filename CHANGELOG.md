# Changelog

All notable changes are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/), and the project
uses [Semantic Versioning](https://semver.org/). While the version is below 1.0, minor releases may contain breaking changes.

## [Unreleased]

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
- Not supported: manifest import, content composition, media upload, adding locales to an existing content item, and approval
  workflows. See the README.
