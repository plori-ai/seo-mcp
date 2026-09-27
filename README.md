# seo-mcp

`seo-mcp` is a Go library and a [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) server for SEO research with the [DataForSEO](https://dataforseo.com/) API. It has five tools: keyword research, keyword metrics, ranked keywords, domain overview, and backlinks overview. The research logic, the tool names, and the JSON field names come from [OpenSEO](https://github.com/every-app/open-seo).

## What seo-mcp does and does not do

`seo-mcp` does stateless research. A tool call sends one or more requests to DataForSEO and returns the result. If you send the same call again, the server does the research again. The project has no storage, no cache, no projects, no rank-tracking schedules, no billing system, and no UI.

You bring your own DataForSEO account. DataForSEO bills each tool call to that account, and one tool call can make several paid requests. A call that fails input validation does not send a request to DataForSEO.

The server uses the operator's DataForSEO credentials for all callers. It does not manage user accounts, quotas, or budgets. The backlinks tool needs Backlinks API access on the DataForSEO account. DataForSEO controls availability, prices, and the returned data. See the [DataForSEO API documentation](https://docs.dataforseo.com/v3/).

## Tools

The endpoint paths are relative to `https://api.dataforseo.com`. A "Labs market" is a country that DataForSEO Labs covers. A "Google Ads market" is a country that has only Google Ads keyword data, for example Iceland (location code 2352).

| Tool | Returns | DataForSEO endpoints |
| --- | --- | --- |
| `research_keywords` | One result for each seed. A result has keyword rows with search volume, CPC, competition, keyword difficulty, intent, and a monthly trend. It also names the last source that the tool called and has a fallback flag. | Labs market: `/v3/dataforseo_labs/google/related_keywords/live`. If the rows have fewer than 5 keywords other than the seed, the tool also calls `/v3/dataforseo_labs/google/keyword_suggestions/live`. If the rows still have fewer than 5, it also calls `/v3/dataforseo_labs/google/keyword_ideas/live`. Google Ads market: only `/v3/keywords_data/google_ads/keywords_for_keywords/live`. |
| `get_keyword_metrics` | Metrics for the given keywords, with optional monthly search volumes. Keywords that DataForSEO has no data for are not in the result. A metric that DataForSEO does not return is `null`. | Labs market: `/v3/dataforseo_labs/google/keyword_overview/live`. Google Ads market: `/v3/keywords_data/google_ads/search_volume/live`. One request for each call. |
| `get_ranked_keywords` | The DataForSEO ranked-keyword items without changes (keyword data and SERP data), the total count, the target, and the scope. | `/v3/dataforseo_labs/google/ranked_keywords/live` |
| `get_domain_overview` | Estimated organic traffic, the organic keyword count, a data-availability flag, the scope, and the fetch time. `backlinks` and `referringDomains` are always `null`. Use `get_backlinks_overview` for backlink counts. | `/v3/dataforseo_labs/google/domain_rank_overview/live` |
| `get_backlinks_overview` | A backlink summary, historical trends, and up to 100 referring domains. | `domain` and `subdomains` scopes: `/v3/backlinks/summary/live`, `/v3/backlinks/history/live`, and `/v3/backlinks/referring_domains/live`. `exact_url` scope: summary and referring domains, with no history. `subfolder` scope: two requests to `/v3/backlinks/backlinks/live`, and no other endpoint. |

Argument limits:

- `research_keywords`: 1 to 5 seeds. `resultLimit` is 150, 300, or 500 keywords for each seed. The default is 150.
- `get_keyword_metrics`: 1 to 700 keywords, each 1 to 80 characters long.
- `get_ranked_keywords`: `limit` from 1 to 100 (default 50) and `offset` from 0 to 1000.

The MCP `tools/list` response and `toolset.Tools()` give the complete argument schemas.

### Markets

The default market is the United States (`locationCode` 2840, `languageCode` `"en"`). A call can select a different country-level location code and language code. If a call sets a location but no language, the tool uses the primary language of that location. The tools reject an unsupported location or language before they send a request.

In a Google Ads market, the keyword tools return search volume, CPC, and trends. Keyword difficulty is `null`. Intent is `"unknown"` in `research_keywords` and `null` in `get_keyword_metrics`. `get_ranked_keywords` and `get_domain_overview` need a Labs market. In a Google Ads market they return an input error.

### Scopes

The scopes are `domain`, `subdomains`, `exact_url`, and `subfolder`. If a call does not set a scope, a bare domain uses `subdomains` and a target with a path uses `subfolder`.

- `get_domain_overview` metrics always include the hostname and all its subdomains. The scope changes only the label in the result.
- `get_backlinks_overview` history always includes subdomains, also for the `domain` scope. The `exact_url` scope has no history. The `subfolder` scope returns only backlink and referring-domain counts, with no rank, history, or referring-domain list.
- For the `domain` and `subfolder` scopes, `get_backlinks_overview` adds a `scopeNote` field that states the limit.

## Install

To build from source, you need Go 1.25.0 or later.

```sh
go install github.com/plori-ai/seo-mcp/cmd/seo-mcp@latest
```

Make sure that the Go install directory is on your `PATH`. This directory is `GOBIN`, or `$(go env GOPATH)/bin` if `GOBIN` is not set.

The [releases page](https://github.com/plori-ai/seo-mcp/releases) has binaries for Linux, macOS, and Windows, on amd64 and arm64. To install a release binary:

1. Download the archive for your platform and `checksums.txt`. The Linux and macOS archives are `.tar.gz` files. The Windows archives are `.zip` files.
2. Compare the SHA-256 checksum of the archive with the value in `checksums.txt`. On Linux, you can use `sha256sum --ignore-missing -c checksums.txt`.
3. Extract the `seo-mcp` binary to a directory on your `PATH`.

To show the version:

```sh
seo-mcp -version
```

A release binary prints its release version. A `go install ...@VERSION` build prints the module version. A build from a local checkout prints `dev`, unless you build with `-ldflags '-X main.version=VERSION'`.

## Credentials

Set one of the two credential forms in the environment of the server.

The first form is one variable:

```sh
export DATAFORSEO_API_KEY='BASE64_OF_LOGIN_COLON_PASSWORD'
```

The value is the base64 encoding of `login:password`, as the DataForSEO dashboard shows it. OpenSEO uses the same form. Do not add a `Basic ` prefix.

The second form is two variables:

```sh
export DATAFORSEO_LOGIN='YOUR_DATAFORSEO_LOGIN'
export DATAFORSEO_PASSWORD='YOUR_DATAFORSEO_API_PASSWORD'
```

If you set both forms, the server uses `DATAFORSEO_API_KEY`. If neither form is complete, the server exits with an error. Do not put credentials in source control.

## Run

By default, the server uses stdio. It writes MCP messages to stdout and diagnostics to stderr.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-http ADDRESS` | empty (stdio) | Serve streamable HTTP at `ADDRESS` on the path `/mcp`. See [HTTP mode](#http-mode). |
| `-location-code CODE` | `2840` | The default DataForSEO location code. |
| `-language-code CODE` | `en` | The default DataForSEO language code. |
| `-version` | | Print the version and exit. |

For example, to make the United Kingdom the default market:

```sh
seo-mcp -location-code 2826 -language-code en
```

A location or language in a tool call overrides the default market. The server checks the default market when it starts, and stops with an error if DataForSEO does not serve that location or language.

## Claude Code

Add the stdio server:

```sh
claude mcp add --env DATAFORSEO_API_KEY=YOUR_BASE64_KEY --scope user --transport stdio seo -- seo-mcp
```

Replace `YOUR_BASE64_KEY` with your key. Claude Code stores the value in your user configuration file, `~/.claude.json`. To use the login and password form, add `DATAFORSEO_LOGIN` and `DATAFORSEO_PASSWORD` with `--env` in the same way.

To keep the key out of the configuration file, use a project `.mcp.json` file. Claude Code expands `${VAR}` references in that file from its environment when it starts the server:

```json
{
  "mcpServers": {
    "seo": {
      "command": "seo-mcp",
      "env": { "DATAFORSEO_API_KEY": "${DATAFORSEO_API_KEY}" }
    }
  }
}
```

Put `seo-mcp` flags after the binary name, for example `-- seo-mcp -location-code 2826 -language-code en`. If `seo-mcp` is not on the `PATH` of Claude Code, use the full path of the binary. To check the connection, run `claude mcp get seo`. For configuration scopes and other options, see the [Claude Code MCP documentation](https://code.claude.com/docs/en/mcp).

## Codex

Add this table to `~/.codex/config.toml`:

```toml
[mcp_servers.seo]
command = "seo-mcp"
env_vars = ["DATAFORSEO_API_KEY", "DATAFORSEO_LOGIN", "DATAFORSEO_PASSWORD"]
tool_timeout_sec = 180
```

Export your credentials before you start Codex. Codex forwards the variables in `env_vars` from its environment to the server, so the file does not contain their values. If `seo-mcp` is not on your `PATH`, set `command` to the full path of the binary. The Codex default tool timeout is 60 seconds. A research call can make several DataForSEO requests, so this example sets 180 seconds. To check the server, run `codex mcp list`. For more options, see the [Codex MCP documentation](https://developers.openai.com/codex/mcp/).

## Library usage

The module has three library packages. No library package imports an MCP SDK.

| Package | Use it when |
| --- | --- |
| [`dataforseo`](https://pkg.go.dev/github.com/plori-ai/seo-mcp/dataforseo) | You need the DataForSEO transport directly: Basic authentication, task envelopes, bounded retries, and classified errors. `Post`, `PostTask`, and `Get` return a task. `FirstResult` decodes the first result of a task. |
| [`seo`](https://pkg.go.dev/github.com/plori-ai/seo-mcp/seo) | You want typed Go requests and results for the five research operations, with market selection and result shaping. |
| [`toolset`](https://pkg.go.dev/github.com/plori-ai/seo-mcp/toolset) | Your application calls tools by name with JSON arguments. `Tools()` returns the tool descriptions and input schemas. `Set.Call` runs a tool through `seo`. |

Typed keyword metrics:

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

A call by tool name, with the same `client` and `ctx` and an import of `github.com/plori-ai/seo-mcp/toolset`:

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

Check `err` before you use the result. Use `errors.As` to examine an error:

- Invalid input returns a `*seo.InputError`. Its message tells the caller what to change.
- A DataForSEO failure returns an error that wraps a `*dataforseo.Error`.
- `Set.Call` with an unknown tool name returns an error that wraps `toolset.ErrUnknownTool`.

`research_keywords` can return a mix of successful and failed seeds. Check the `ok` field of each seed result. By default, each HTTP request to DataForSEO has a 60-second timeout. `dataforseo.Client.HTTPClient` overrides it. To limit the time of a complete call, set a deadline on the context. The transport retries an HTTP 5xx response up to two times. It does not retry an HTTP 4xx response or a failed task status.

## OpenSEO compatibility

The five tool names, the argument names, and the result field names are the same as in OpenSEO. For example, `get_keyword_metrics` returns `search_volume` and `research_keywords` returns `searchVolume`, as in OpenSEO. Nullable metrics and nested DataForSEO rows keep their JSON types.

These differences are intentional:

- No tool has a `projectId` argument, and results have no `meta` field.
- Credentials and the default market come from the server configuration or from the library caller. There is no project lookup.
- There is no OpenSEO credit accounting and no cached research. Each call uses your DataForSEO account directly.
- A successful MCP result has the result JSON in `structuredContent` and the same JSON as text in a text content block. OpenSEO returns Markdown tables in the text block.
- The tool annotations mark each tool as read-only and open-world. Read-only does not mean free: each call spends DataForSEO balance.
- A tool error contains one of these: an input correction, a short error message, or the task status message from DataForSEO. Failed authentication and rate limits return a short error message. A tool error never contains a raw HTTP response body.

`seo-mcp` has only these five tools. It does not port the other OpenSEO features, such as projects and scheduled rank tracking.

## HTTP mode

To serve streamable HTTP on the local host:

```sh
seo-mcp -http 127.0.0.1:8080
```

The endpoint is `http://127.0.0.1:8080/mcp`. HTTP mode is stateless. These rules apply:

- On a loopback address with no `SEO_MCP_TOKEN`, the server accepts requests without authentication. Other processes on the same host can use it.
- On a non-loopback address, the server does not start without `SEO_MCP_TOKEN`. An address with no host, such as `:8080`, is a non-loopback address.
- If you set `SEO_MCP_TOKEN`, each request must have exactly one `Authorization: Bearer <token>` header. This also applies to loopback addresses. The server returns HTTP 401 for a missing or incorrect token.

To serve on all interfaces:

```sh
export SEO_MCP_TOKEN='YOUR_RANDOM_SECRET_TOKEN'
seo-mcp -http 0.0.0.0:8080
```

The server compares tokens in constant time. It rejects cross-origin requests from browsers. The server serves plain HTTP. For remote access, put HTTPS in front of it with a reverse proxy or use a different encrypted connection. Give access only to trusted callers, because each authenticated caller spends the same DataForSEO balance. The server has no per-user authorization and no rate limit.

The reverse proxy must forward the `Authorization` header and must block direct access to the backend listener. If the proxy connects to a loopback listener, it must send a loopback `Host` header, such as `127.0.0.1:8080` or `localhost:8080`. For a request on a loopback connection, the MCP SDK returns HTTP 403 if the `Host` header is not a loopback name. This check prevents DNS rebinding.

For a Codex connection over HTTP, use this table instead of the stdio table:

```toml
[mcp_servers.seo]
url = "http://127.0.0.1:8080/mcp"
bearer_token_env_var = "SEO_MCP_TOKEN"
tool_timeout_sec = 180
```

The client and the server must use the same token. Keep the DataForSEO credentials on the server. To report a vulnerability, see [SECURITY.md](SECURITY.md).

## Contributing

[CONTRIBUTING.md](CONTRIBUTING.md) describes the package boundaries, the local checks, and the rules for test fixtures. [CHANGELOG.md](CHANGELOG.md) lists the changes in each release.

## License and attribution

`seo-mcp` uses the [MIT License](LICENSE). Plori ported the research logic, the tool contracts, and the market data from [OpenSEO](https://github.com/every-app/open-seo) by Ben Senescu. OpenSEO also uses the MIT License. The [LICENSE](LICENSE) file has two copyright lines: one for Plori and one for Ben Senescu (OpenSEO). DataForSEO is a separate data provider, and its API terms and prices apply.
