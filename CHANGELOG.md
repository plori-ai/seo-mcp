# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and releases use [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- DataForSEO transport with Basic authentication, task-envelope validation, classified errors, and bounded HTTP 5xx retries.
- Typed Go operations for keyword research, keyword metrics, ranked keywords, domain overviews, and backlink overviews.
- Named tool definitions and JSON schemas compatible with the corresponding OpenSEO research tools, without project arguments or metadata.
- MCP server with stdio and streamable HTTP transports, configurable default market, and release version reporting.
- Bearer-token protection for HTTP requests and refusal of unauthenticated non-loopback listeners.
- Synthetic transport and research fixtures, MCP transport tests, and library examples.
- CI for Go 1.25 and 1.26, and GoReleaser configuration for Linux, macOS, and Windows archives with checksums.

[Unreleased]: https://github.com/plori-ai/seo-mcp/commits/main
