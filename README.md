# seo-mcp

`seo-mcp` is a Go library and [Model Context Protocol](https://modelcontextprotocol.io/) server for SEO research through [DataForSEO](https://dataforseo.com/). It provides five tools for keyword research, keyword metrics, ranked keywords, domain overviews, and backlink overviews. The research behavior and JSON field names are ported from [OpenSEO](https://github.com/every-app/open-seo).

## Scope and costs

The tools perform stateless research using your own DataForSEO account. Every tool call that reaches DataForSEO is billed by DataForSEO; one call can make several paid API requests. Repeating a call performs the research again. `seo-mcp` has no storage, caching, projects, rank tracking schedules, billing system, or UI.

The server uses the operator's DataForSEO credentials for every caller. It does not manage user accounts, quotas, or budgets. Backlink tools require Backlinks API access on that account. Availability, charges, and returned data depend on DataForSEO; see its [API documentation](https://docs.dataforseo.com/v3/).

## Tools

All endpoint paths below are relative to `https://api.dataforseo.com`. Fallback endpoints are used only when needed for the selected market or the results returned.

| Tool | Returns | DataForSEO endpoints used |
| --- | --- | --- |
| `research_keywords` | Per-seed keyword rows with volume, CPC, competition, difficulty, intent, trends, source, and fallback status. | `/v3/dataforseo_labs/google/related_keywords/live`, `/v3/dataforseo_labs/google/keyword_suggestions/live`, `/v3/dataforseo_labs/google/keyword_ideas/live`; `/v3/keywords_data/google_ads/keywords_for_keywords/live` for markets served by Google Ads. |
| `get_keyword_metrics` | Metrics for supplied keywords, with optional monthly search volumes. Missing provider metrics remain `null`. | `/v3/dataforseo_labs/google/keyword_overview/live`; `/v3/keywords_data/google_ads/search_volume/live` for markets served by Google Ads. |
| `get_ranked_keywords` | Raw ranked-keyword items with nested keyword and SERP data, total count, target, and scope. | `/v3/dataforseo_labs/google/ranked_keywords/live` |
| `get_domain_overview` | Estimated organic traffic, organic keyword count, data availability, scope, and fetch time. Backlink counts are `null`; use `get_backlinks_overview` for them. | `/v3/dataforseo_labs/google/domain_rank_overview/live` |
| `get_backlinks_overview` | Backlink summary, available historical trends, and up to 100 referring domains. Subfolder results contain filtered counts. | `/v3/backlinks/summary/live`, `/v3/backlinks/history/live`, `/v3/backlinks/referring_domains/live`; `/v3/backlinks/backlinks/live` for subfolder counts. |

Keyword research accepts one to five seeds and a `resultLimit` of 150, 300, or 500 per seed. Keyword metrics accepts up to 700 keywords. Ranked keywords accepts a `limit` from 1 to 100 and an `offset` from 0 to 1000. Discover the complete argument schemas through MCP `tools/list` or `toolset.Tools()`.

The default market is the United States (`locationCode: 2840`, `languageCode: "en"`). Calls can select supported country and language codes. A different location with no language selects that location's primary language. Domain analytics require a DataForSEO Labs market; keyword tools also support designated Google Ads markets.

Scopes are `domain`, `subdomains`, `exact_url`, and `subfolder`. Domain-overview metrics always cover the hostname and its subdomains, even when the result labels a narrower scope. Backlink history includes subdomains for domain scopes; exact-page results have no history, and subfolders have neither history nor a referring-domain breakdown. `scopeNote` describes the relevant backlink limitation.

## Install

Building from source requires Go 1.25.0 or later:

```sh
go install github.com/plori-ai/seo-mcp/cmd/seo-mcp@latest
```

Make sure the Go install directory (`GOBIN`, or `$(go env GOPATH)/bin` when unset) is on your `PATH`.

Release binaries are published on the [releases page](https://github.com/plori-ai/seo-mcp/releases) for Linux, macOS, and Windows, on amd64 and arm64. Download the matching archive and `checksums.txt`, verify the archive's SHA-256 checksum, and extract the binary into a directory on your `PATH`. Unix archives use `.tar.gz`; Windows archives use `.zip`.

```sh
seo-mcp -version
```

Release binaries print their release version. Source builds print `dev` unless built with `-ldflags '-X main.version=VERSION'`.

## Credentials and default market

Set one of these credential forms in the server's environment:

```sh
export DATAFORSEO_API_KEY='BASE64_OF_LOGIN_COLON_PASSWORD'
```

`DATAFORSEO_API_KEY` is the base64 encoding of `login:password`, as shown by the DataForSEO dashboard. It is the same credential form used by OpenSEO. Do not prepend `Basic `.

Alternatively:

```sh
export DATAFORSEO_LOGIN='YOUR_DATAFORSEO_LOGIN'
export DATAFORSEO_PASSWORD='YOUR_DATAFORSEO_API_PASSWORD'
```

`DATAFORSEO_API_KEY` takes precedence when both forms are present. The server exits with an error if neither form is complete. Keep credentials out of source control.

The binary serves stdio by default. Its stdout carries MCP messages; diagnostics go to stderr. Set a different default market with:

```sh
seo-mcp -location-code 2826 -language-code en
```

Explicit tool arguments override the default market. Run `seo-mcp -help` for all flags.

## Claude Code

With credentials exported in the environment from which you launch Claude Code, register the stdio server:

```sh
claude mcp add --scope user --transport stdio seo -- seo-mcp
```

Use the full binary path if `seo-mcp` is not on Claude Code's `PATH`. Flags go after the binary, for example `-- seo-mcp -location-code 2826 -language-code en`. Check the connection with `claude mcp list`. See the [Claude Code MCP documentation](https://code.claude.com/docs/en/mcp) for configuration scopes and environment settings.

## Codex

Add this to `~/.codex/config.toml`:

```toml
[mcp_servers.seo]
command = "seo-mcp"
args = []
env_vars = ["DATAFORSEO_API_KEY", "DATAFORSEO_LOGIN", "DATAFORSEO_PASSWORD"]
tool_timeout_sec = 180
```

Export your chosen credentials before starting Codex. `env_vars` forwards them to the server without storing their values in this file. Use an absolute `command` path when needed. The timeout allows research calls that make several provider requests. See the [Codex MCP documentation](https://developers.openai.com/codex/mcp/).

## Library usage

The module has three library packages. None imports an MCP SDK.

| Package | Use it when |
| --- | --- |
| [`dataforseo`](https://pkg.go.dev/github.com/plori-ai/seo-mcp/dataforseo) | You need the DataForSEO transport directly: Basic authentication, task envelopes, bounded retries, and classified errors. `Post`, `PostTask`, and `Get` return tasks; `FirstResult` decodes the first result. |
| [`seo`](https://pkg.go.dev/github.com/plori-ai/seo-mcp/seo) | You want typed Go requests and results for the five research operations, including market selection and response shaping. |
| [`toolset`](https://pkg.go.dev/github.com/plori-ai/seo-mcp/toolset) | Your application calls tools by name and passes JSON arguments. `Tools()` provides the descriptions and input schemas; `Set.Call` dispatches to `seo`. |

For typed keyword metrics:

```go
package main

import (
    "context"
    "encoding/json"
    "log"
    "os"
    "time"

    "github.com/plori-ai/seo-mcp/dataforseo"
    "github.com/plori-ai/seo-mcp/seo"
)

func main() {
    api := dataforseo.New(os.Getenv("DATAFORSEO_API_KEY"))
    client := seo.New(api, seo.WithDefaultMarket(seo.Market{
        LocationCode: 2840,
        LanguageCode: "en",
    }))
    ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
    defer cancel()

    result, err := client.KeywordMetrics(ctx, seo.KeywordMetricsRequest{
        Keywords: []string{"technical seo", "site audit"},
        SortBy:   "search_volume",
    })
    if err != nil {
        log.Fatal(err)
    }
    if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
        log.Fatal(err)
    }
}
```

For named dispatch, import `github.com/plori-ai/seo-mcp/toolset` and use the same `client` and `ctx`:

```go
set := toolset.New(client)
result, err := set.Call(ctx, "get_ranked_keywords", json.RawMessage(`{
    "target": "example.com",
    "scope": "subdomains",
    "locationCode": 2840,
    "languageCode": "en",
    "limit": 10
}`))
```

Handle `err` before encoding the result. Invalid inputs use `*seo.InputError`; provider failures wrap `*dataforseo.Error`. Use `errors.As` to inspect them. `research_keywords` can return a mix of successful and failed seeds, so also check each result's `ok` field. Library callers supply their own context deadlines and decide how to present errors. The transport retries HTTP 5xx responses twice; it does not retry HTTP 4xx or failed task statuses.

## OpenSEO compatibility

The five tool names, argument names, and result field names follow OpenSEO. For example, `get_keyword_metrics` returns `search_volume` while `research_keywords` returns `searchVolume`; the library preserves that distinction. Nullable metrics and nested provider rows retain their JSON types.

The differences are intentional:

- `projectId` is not an argument, and results omit `meta` and project URLs.
- Credentials and default markets come from the caller or server configuration, with no project lookup.
- There is no OpenSEO credit accounting or cached research. Calls use your DataForSEO account directly.
- Successful MCP results contain the same JSON in `structuredContent` and a text content block. The text block is JSON instead of OpenSEO's formatted tables.
- Tool annotations mark research as read-only and open-world. Read-only does not mean free: calls spend DataForSEO balance.
- Returned tool errors expose input corrections or a short provider-error category, instead of raw provider response bodies.

This server does not implement OpenSEO's project, saved-keyword, audit, or scheduled rank-tracking tools.

## HTTP mode and security

To serve streamable HTTP locally:

```sh
seo-mcp -http 127.0.0.1:8080
```

The endpoint is `http://127.0.0.1:8080/mcp`. HTTP handling is stateless. If `SEO_MCP_TOKEN` is set to a nonempty token, **every request** must include `Authorization: Bearer <token>`, including requests to loopback listeners.

```sh
export SEO_MCP_TOKEN='YOUR_RANDOM_SECRET_TOKEN'
seo-mcp -http 0.0.0.0:8080
```

The server refuses a non-loopback listener without a token. It compares tokens in constant time and rejects cross-origin browser requests. Remote access needs HTTPS through a reverse proxy or another encrypted connection; the binary itself serves plain HTTP. Restrict access to trusted callers, since every authenticated caller can spend the same DataForSEO balance. There is no per-user authorization or rate limit. A reverse proxy must preserve the bearer header and restrict access to the backend listener. When proxying to loopback, send a loopback backend `Host` header: the MCP SDK rejects other hostnames on loopback connections to prevent DNS rebinding.

For Codex HTTP connections, replace the stdio entry with:

```toml
[mcp_servers.seo]
url = "http://127.0.0.1:8080/mcp"
bearer_token_env_var = "SEO_MCP_TOKEN"
tool_timeout_sec = 180
```

The client and server must receive the same token. Keep DataForSEO credentials on the server. See [SECURITY.md](SECURITY.md) for vulnerability reporting.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for package boundaries, local checks, and synthetic test fixtures. Changes are recorded in [CHANGELOG.md](CHANGELOG.md).

## License and attribution

`seo-mcp` is licensed under the [MIT License](LICENSE). Research logic, tool contracts, and market data are ported from [OpenSEO](https://github.com/every-app/open-seo), also MIT licensed, by Ben Senescu and contributors. The license retains the upstream attribution. DataForSEO is the external data provider; its API terms and charges apply separately.
