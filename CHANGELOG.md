# Changelog

This file lists the notable changes to this project.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Version numbers follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-09-27

### Added

- Five MCP tools, ported from OpenSEO with the same tool names, argument names, and result field names: `research_keywords`, `get_keyword_metrics`, `get_ranked_keywords`, `get_domain_overview`, and `get_backlinks_overview`. The tools have no `projectId` argument and no `meta` field.
- The `seo-mcp` MCP server, with stdio and streamable HTTP transports, `-location-code` and `-language-code` flags for the default market, and a `-version` flag.
- Bearer-token authentication with `SEO_MCP_TOKEN` for HTTP mode. The server does not start on a non-loopback address without a token.
- The `dataforseo` package: a DataForSEO v3 transport with Basic authentication, task-envelope checks, classified errors, and up to two retries of HTTP 5xx responses.
- The `seo` package: typed Go requests and results for the five research operations, with market selection and validation.
- The `toolset` package: tool definitions with JSON schemas and dispatch by tool name, with no MCP SDK dependency.
- Tests with synthetic DataForSEO responses, MCP transport tests, and library examples.
- CI on Go 1.25 and Go 1.26, and GoReleaser archives for Linux, macOS, and Windows with SHA-256 checksums.

[Unreleased]: https://github.com/plori-ai/seo-mcp/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/plori-ai/seo-mcp/releases/tag/v0.1.0
