# Changelog

This file lists the notable changes to this project.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Version numbers follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Two MCP tools that use DataForSEO task queues, ported from OpenSEO with the same tool names, argument names, and result field names:
  - `get_business_reviews`: the Google reviews of one business, from `/v3/business_data/google/reviews/task_post` and `task_get`. With `includeOtherSources`, it uses the `extended_reviews` endpoints and also collects reviews from other sites.
  - `get_business_updates`: the Google Business posts of one business, from `/v3/business_data/google/my_business_updates/task_post` and `task_get`.
- Each call posts a high-priority task and waits up to 20 seconds for it. If the task is still running, the result has `status: "processing"` and a `taskId`. A call with only `taskId` collects the task and posts no new task. [docs/async-tasks.md](docs/async-tasks.md) explains the design.
- `seo.Client.BusinessReviews`, `seo.Client.BusinessUpdates`, their request and result types, `seo.WithTaskPolling`, and `seo.TaskError`, which carries the task ID of a failure after DataForSEO created a task.

### Changed

- `dataforseo.Client.PostTask` no longer retries an HTTP 5xx response. DataForSEO charges when it creates a task, and a 5xx response does not prove that it did not create one. `PostTask` also returns an error when the created task has no ID.
- The MCP error for an HTTP 5xx response to a task_post request says that DataForSEO may have billed the task. The error for a failure after DataForSEO created a task contains the `taskId`.

## [0.2.0] - 2026-09-27

### Added

- Eight MCP tools, ported from OpenSEO with the same tool names, argument names, and result field names. Each tool sends synchronous DataForSEO requests and keeps no state:
  - `find_serp_competitors`: the domains that rank in Google results for a set of keywords, from `/v3/dataforseo_labs/google/serp_competitors/live`. It needs a DataForSEO Labs market.
  - `get_backlinks_profile`: one page of detailed backlink rows, with filters, sort, and pages, from `/v3/backlinks/backlinks/live`. It needs Backlinks API access on the DataForSEO account.
  - `search_local_businesses`: business listings near a coordinate, from `/v3/business_data/business_listings/search/live`.
  - `list_business_categories`: Google Business category names and business counts, from `GET /v3/business_data/business_listings/categories`. DataForSEO does not charge for this call.
  - `get_business_profile`: one Google Business Profile, from `/v3/business_data/google/my_business_info/live`.
  - `get_google_business_questions`: the questions and answers of one business, from `/v3/business_data/google/questions_and_answers/live`.
  - `get_local_serp_results`: one Google Maps or Local Finder result list near a coordinate, from `/v3/serp/google/maps/live/advanced` or `/v3/serp/google/local_finder/live/advanced`.
  - `get_local_rank_grid`: the Google Maps rank of one business on a 3 x 3 or 5 x 5 grid of points. It sends one `/v3/serp/google/maps/live/advanced` request for each point, 9 or 25 requests for each call.
- Typed Go requests and results for the eight operations in the `seo` package, and their tool definitions in the `toolset` package.
- Tests with synthetic DataForSEO responses for the eight tools.

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

[Unreleased]: https://github.com/plori-ai/seo-mcp/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/plori-ai/seo-mcp/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/plori-ai/seo-mcp/releases/tag/v0.1.0
