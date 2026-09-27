# seo-mcp

`seo-mcp` is a Go library and a [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) server for SEO research with the [DataForSEO](https://dataforseo.com/) API. The server includes fifteen tools. Seven tools do keyword, domain, competitor, and backlink research. Eight tools return local business data and local Google Maps results. The research logic, the tool names, and the JSON field names come from [OpenSEO](https://github.com/every-app/open-seo).

## What seo-mcp does and does not do

`seo-mcp` does stateless research. A tool call sends one or more requests to DataForSEO and returns the result. If you send the same call again, the server does the research again. The project has no storage, no cache, no projects, no rank-tracking schedules, no billing system, and no UI.

Two tools, `get_business_reviews` and `get_business_updates`, use DataForSEO task queues, because DataForSEO has no live endpoint for this data. These tools post a task, wait up to 20 seconds for the result, and return a task ID if the task is still running. A later call with that task ID collects the result at no extra charge. The server does not store task IDs. See [Queued tasks](#queued-tasks). All other tools use DataForSEO endpoints that return the result in the same request.

You bring your own DataForSEO account. DataForSEO bills each tool call to that account, and one tool call can make several paid requests. A call that fails input validation does not send a request to DataForSEO. [Requests and costs](#requests-and-costs) shows the number of requests for each tool.

The server uses the operator's DataForSEO credentials for all callers. It does not manage user accounts, quotas, or budgets. The two backlinks tools, `get_backlinks_overview` and `get_backlinks_profile`, need Backlinks API access on the DataForSEO account. DataForSEO controls availability, prices, and the returned data. See the [DataForSEO API documentation](https://docs.dataforseo.com/v3/).

## Tools

The endpoint paths are relative to `https://api.dataforseo.com`. All endpoints use `POST`, except where the table shows `GET`. A "Labs market" is a country that DataForSEO Labs covers. A "Google Ads market" is a country that has only Google Ads keyword data, for example Iceland (location code 2352).

### Keyword, domain, competitor, and backlink tools

| Tool | Returns | DataForSEO endpoints |
| --- | --- | --- |
| `research_keywords` | One result for each seed. A result has keyword rows with search volume, CPC, competition, keyword difficulty, intent, and a monthly trend. It also names the last source that the tool called and has a fallback flag. | Labs market: `/v3/dataforseo_labs/google/related_keywords/live`. If the rows have fewer than 5 keywords other than the seed, the tool also calls `/v3/dataforseo_labs/google/keyword_suggestions/live`. If the rows still have fewer than 5, it also calls `/v3/dataforseo_labs/google/keyword_ideas/live`. Google Ads market: only `/v3/keywords_data/google_ads/keywords_for_keywords/live`. |
| `get_keyword_metrics` | Metrics for the given keywords, with optional monthly search volumes. Keywords that DataForSEO has no data for are not in the result. A metric that DataForSEO does not return is `null`. | Labs market: `/v3/dataforseo_labs/google/keyword_overview/live`. Google Ads market: `/v3/keywords_data/google_ads/search_volume/live`. One request for each call. |
| `get_ranked_keywords` | The DataForSEO ranked-keyword items without changes (keyword data and SERP data), the total count, the target, and the scope. | `/v3/dataforseo_labs/google/ranked_keywords/live` |
| `get_domain_overview` | Estimated organic traffic, the organic keyword count, a data-availability flag, the scope, and the fetch time. `backlinks` and `referringDomains` are always `null`. Use `get_backlinks_overview` for backlink counts. | `/v3/dataforseo_labs/google/domain_rank_overview/live` |
| `get_backlinks_overview` | A backlink summary, historical trends, and up to 100 referring domains. | `domain` and `subdomains` scopes: `/v3/backlinks/summary/live`, `/v3/backlinks/history/live`, and `/v3/backlinks/referring_domains/live`. `exact_url` scope: summary and referring domains, with no history. `subfolder` scope: two requests to `/v3/backlinks/backlinks/live`, and no other endpoint. |
| `find_serp_competitors` | The domains that rank in Google results for a set of keywords. Each row is a DataForSEO competitor item without changes, with fields such as `keywords_count`, `avg_position`, `visibility`, and `etv` (estimated traffic). | `/v3/dataforseo_labs/google/serp_competitors/live` |
| `get_backlinks_profile` | One page of backlink rows. A row has the linking domain and URL, the target URL, and the anchor text. It also has the dofollow flag, the rel attributes, the link rank, the domain rank, and the spam score. It also has the first-seen date, the last-seen date, and the lost and broken flags. The page also has the total count and a `hasMore` flag. | `/v3/backlinks/backlinks/live` |

### Local business and local search tools

| Tool | Returns | DataForSEO endpoints |
| --- | --- | --- |
| `search_local_businesses` | Business listings near a coordinate. A row has the title, description, category, additional categories, address, phone, URL, and domain. It also has the rating, the claimed flag, the CID, the place ID, the latitude and longitude, the photo count, and the check URL. A row has no other DataForSEO fields. | `/v3/business_data/business_listings/search/live` |
| `list_business_categories` | Google Business category names and the number of businesses in each, in descending order of that number. If the call sets `query`, the result has only the names that contain it. The match ignores case. | `GET /v3/business_data/business_listings/categories`. The tool requests the full list in each call and filters it locally. |
| `get_business_profile` | All DataForSEO fields of one Google Business Profile, or `profile: null` if no business matches. | `/v3/business_data/google/my_business_info/live` |
| `get_google_business_questions` | The answered and unanswered questions of one business. A question has its text, author, and time, and an `items` list of answers with the same fields. `items` is `null` if DataForSEO returns no answer list. | `/v3/business_data/google/questions_and_answers/live` |
| `get_business_reviews` | The Google reviews of one business. A review has the rank, the time, the rating, the text and its original language, the author and the author's review and photo counts, the review highlights, the source site, and the owner's reply. `totals` has the business title, the total review count, the rating, the CID, and the place ID. If the task is still running, the result has only `status: "processing"` and `taskId`. | `/v3/business_data/google/reviews/task_post`, then `GET /v3/business_data/google/reviews/task_get/{id}`. With `includeOtherSources`: the same two endpoints under `extended_reviews`. |
| `get_business_updates` | The posts (updates, offers, and events) of one Google Business Profile. A post has the rank, the author, the date, the text or snippet, the URL, and the links. If the task is still running, the result has only `status: "processing"` and `taskId`. | `/v3/business_data/google/my_business_updates/task_post`, then `GET /v3/business_data/google/my_business_updates/task_get/{id}` |
| `get_local_serp_results` | One Google Maps or Local Finder result list near a coordinate. A row has the rank, the business name, CID, and place ID, the rating, the categories, the contact data, the opening hours, and the coordinates. | `searchType` `maps` (default): `/v3/serp/google/maps/live/advanced`. `searchType` `local_finder`: `/v3/serp/google/local_finder/live/advanced`. |
| `get_local_rank_grid` | The Google Maps rank of one business at each point of a square grid around a center coordinate. A point has the rank, the result count, and the first result. The rank is `null` if the business is not in the first 20 results. The result also has a summary and the matched business. The summary counts the points searched, the points found, and the points in the top 3 and the top 10. It also has the average rank. The points are in rows from north to south. | `/v3/serp/google/maps/live/advanced`, one request for each grid point, with a depth of 20 results |

### Argument limits

- `research_keywords`: 1 to 5 seeds. `resultLimit` is 150, 300, or 500 keywords for each seed. The default is 150.
- `get_keyword_metrics`: 1 to 700 keywords, each 1 to 80 characters long.
- `get_ranked_keywords`: `limit` from 1 to 100 (default 50) and `offset` from 0 to 1000.
- `find_serp_competitors`: 1 to 100 keywords, each 1 to 120 characters long. `limit` from 1 to 100 (default 50) and `offset` from 0 to 1000. The default `resultTypes` are `organic` and `local_pack`. The tool applies `excludeDomains` and `sortBy` to the rows that DataForSEO returns. Thus the result can have fewer rows than `limit`.
- `get_backlinks_profile`: `page` from 1, and `pageSize` 50, 100, or 200 (default 100). The default sort is `firstSeen`, descending. The default `mode` is `one_per_domain`, which returns one link for each referring domain. `as_is` returns each backlink. `hideSpam` is `true` by default and keeps only links with a spam score of 40 or less. A call can have a maximum of 8 filter conditions. Each `include` or `exclude` term, each numeric bound, and each other filter is one condition. `hideSpam` is one condition, and the `subfolder` scope uses four.
- `search_local_businesses`: The call must set `near`. `near.radiusKm` is from 1 to 100000, and the tool rounds it to whole kilometers. `limit` from 1 to 50 (default 20) and `offset` from 0 to 1000. `categories` has 1 to 10 category names. Use `list_business_categories` to find the names.
- `list_business_categories`: `limit` from 1 to 200 (default 50).
- `get_business_profile`: exactly one of `businessName`, `cid`, or `placeId`. `near` is optional. `near.radiusKm` is from 0.2 to 199 (default 10). If the call sets `near`, the tool ignores `locationCode`.
- `get_google_business_questions`: exactly one of `businessName`, `cid`, or `placeId`, and `near`. `near.radiusKm` is from 1 to 100000, but the request to DataForSEO uses a maximum radius of 199,999 meters. `depth` from 1 to 100 (default 20).
- `get_business_reviews`: exactly one of `businessName`, `cid`, or `placeId`, or only `taskId`. `near` and `locationCode` work as in `get_business_profile`. `depth` from 10 to 200 (default 20). `sortBy` is `newest` (default), `highest_rating`, `lowest_rating`, or `relevant`. If `includeOtherSources` is `true`, the tool also collects reviews from other sites, and it ignores `sortBy`.
- `get_business_updates`: exactly one of `businessName`, `cid`, or `placeId`, or only `taskId`. `near` and `locationCode` work as in `get_business_profile`. `depth` from 10 to 100 (default 10).
- `get_local_serp_results`: `keyword` from 1 to 120 characters. `depth` from 1 to 100 (default 20). `near.zoom` from 4 to 18. `device` is `mobile` (default) or `desktop`.
- `get_local_rank_grid`: `target` has one or more of `cid`, `placeId`, and `name`. A row matches if its CID or place ID is equal to the given value, or if its title contains `name`. The name match ignores case. The first row that matches gives the rank. `gridSize` is 3 (default) or 5. `spacingKm` is from 0.25 to 10 (default 2). If the call does not set `zoom`, the tool calculates one zoom level for all points from `spacingKm` and the center latitude.

The MCP `tools/list` response and `toolset.Tools()` give the complete argument schemas.

### Empty results and failed points

If DataForSEO reports no search results, `search_local_businesses`, `get_google_business_questions`, and `get_local_serp_results` return an empty list. `get_business_profile` returns `profile: null`. `get_business_reviews` returns `reviews: []` and `totals: null`, and `get_business_updates` returns `updates: []`.

In `get_local_rank_grid`, a point with a failed request has `error: true` and `rank: null`. If all points fail, the tool returns the last error. An HTTP 401 response from DataForSEO stops the grid, and the tool returns an error. A cancelled call also returns an error. In these two cases, the tool returns no points, but DataForSEO can bill the requests that it received.

### Requests and costs

DataForSEO sets the price of each request. For the current prices, see [DataForSEO pricing](https://dataforseo.com/pricing). This list gives the number of requests for each tool, and the arguments that change the price:

- `research_keywords`: up to three requests for each seed. The DataForSEO Labs endpoints charge for each returned row, so a larger `resultLimit` can cost more.
- `get_backlinks_overview`: up to three requests.
- `get_keyword_metrics`, `get_ranked_keywords`, and `get_domain_overview`: one request.
- `find_serp_competitors`, `get_backlinks_profile`, and `search_local_businesses`: one request. These endpoints charge for each returned row, so a larger `limit` or `pageSize` can cost more.
- `list_business_categories`: one request. DataForSEO does not charge for it.
- `get_business_profile`: one request. DataForSEO can bill a call that finds no business.
- `get_google_business_questions`: one request. DataForSEO charges for each block of 20 returned questions, so a larger `depth` can cost more.
- `get_local_serp_results`: one request. A Google Maps request has one price for up to 100 results. A Local Finder request has a price for each 10 mobile results or 20 desktop results, so a larger `depth` can increase its price.
- `get_local_rank_grid`: one Google Maps request for each grid point. That is 9 requests for `gridSize` 3, and 25 requests for `gridSize` 5. The tool sends a maximum of three requests at the same time. DataForSEO can bill a point that fails.
- `get_business_reviews`: one high-priority task. High priority costs two times the normal price. DataForSEO charges for each block of 10 returned reviews, so a larger `depth` costs more: at the time of writing, about $0.003 for the default 20 reviews and $0.03 for 200. With `includeOtherSources`, DataForSEO charges for each block of 20 reviews, and its task_post documentation lists three times the standard rate for a task with `businessName` and two times for a task with `cid` or `placeId`.
- `get_business_updates`: one high-priority task. DataForSEO charges for the task and for each block of 10 returned posts: at the time of writing, about $0.0045 for the default 10 posts.
- The task_get requests that collect a result are free. A call with `taskId` costs nothing. A call without `taskId` always creates and pays for a new task, also when an earlier call for the same business is still running.

### Markets

The default market is the United States (`locationCode` 2840, `languageCode` `"en"`). The two keyword tools, `get_ranked_keywords`, `get_domain_overview`, and `find_serp_competitors` accept a different country-level location code and language code. If a call sets a location but no language, the tool uses the primary language of that location. These tools reject an unsupported location or language before they send a request.

In a Google Ads market, the keyword tools return search volume, CPC, and trends. Keyword difficulty is `null`. Intent is `"unknown"` in `research_keywords` and `null` in `get_keyword_metrics`. `get_ranked_keywords`, `get_domain_overview`, and `find_serp_competitors` need a Labs market. In a Google Ads market they return an input error. The `market.country` argument of `find_serp_competitors` is an older selector, and it accepts only the United States.

The two backlinks tools and `list_business_categories` do not use a market. `get_business_profile`, `get_business_reviews`, and `get_business_updates` use `locationCode` if the call does not set `near`. These tools send this code to DataForSEO without a check, so they also accept city location codes. The other local tools use a coordinate, not a location code. In `get_business_profile`, `get_google_business_questions`, `get_business_reviews`, `get_business_updates`, `get_local_serp_results`, and `get_local_rank_grid`, `languageCode` can be any language that DataForSEO supports. The default is the language of the default market.

### Queued tasks

`get_business_reviews` and `get_business_updates` work in this sequence:

1. The tool posts one DataForSEO task at high priority. DataForSEO charges for the task at this step.
2. The tool checks the task every 4 seconds for up to 20 seconds.
3. If the task is complete, the tool returns `status: "completed"`, the `taskId`, and the rows.
4. If the task is still running, the tool returns only `status: "processing"` and `taskId`. Call the tool again after 30 to 60 seconds with only `taskId`. That call posts no task and costs nothing. It checks the task at once and then for up to 20 seconds again.

DataForSEO keeps a task result for 30 days. A reviews task ID has the form `google:<id>` or `extended:<id>`, and an updates task ID is the bare DataForSEO ID. Pass it back to the same tool without changes.

The server never sends a task_post request again. If DataForSEO returns an HTTP 5xx response to a task_post request, it can have created and billed the task, so the tool returns an error. If a failure occurs after DataForSEO created the task, for example a cancelled call or a failed task_get request, the error text contains the `taskId` to collect the task. [docs/async-tasks.md](docs/async-tasks.md) explains the design.

### Scopes

The scopes are `domain`, `subdomains`, `exact_url`, and `subfolder`. If a call does not set a scope, a bare domain uses `subdomains` and a target with a path uses `subfolder`.

- `get_domain_overview` metrics always include the hostname and all its subdomains. The scope changes only the label in the result.
- `get_backlinks_overview` history always includes subdomains, also for the `domain` scope. The `exact_url` scope has no history. The `subfolder` scope returns only backlink and referring-domain counts, with no rank, history, or referring-domain list.
- For the `domain` and `subfolder` scopes, `get_backlinks_overview` adds a `scopeNote` field that states the limit.
- In `get_backlinks_profile`, the `domain` scope does not include subdomains.
- In the two backlinks tools, `page` is an older name for `exact_url`. An `exact_url` target cannot have a query string or a fragment.

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
| [`dataforseo`](https://pkg.go.dev/github.com/plori-ai/seo-mcp/dataforseo) | You need the DataForSEO transport directly: Basic authentication, task envelopes, bounded retries, and classified errors. `Post`, `PostTask`, and `Get` return a task. `PostTask` never retries. `FirstResult` decodes the first result of a task. |
| [`seo`](https://pkg.go.dev/github.com/plori-ai/seo-mcp/seo) | You want typed Go requests and results for the fifteen research operations, with market selection and result shaping. |
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
- A failure after DataForSEO created a queued task returns a `*seo.TaskError`. Its `TaskID` collects the task in a later call. It wraps the cause, so `errors.Is` and `errors.As` also find the context error or the `*dataforseo.Error`.

`research_keywords` can return a mix of successful and failed seeds. Check the `ok` field of each seed result. By default, each HTTP request to DataForSEO has a 60-second timeout. `dataforseo.Client.HTTPClient` overrides it. To limit the time of a complete call, set a deadline on the context. The transport retries an HTTP 5xx response up to two times, except for a task_post request, which it never sends again. It does not retry an HTTP 4xx response or a failed task status. `seo.WithTaskPolling` sets how long `BusinessReviews` and `BusinessUpdates` wait for a queued task. A wait of zero returns the task ID of a new task at once.

## OpenSEO compatibility

The fifteen tool names, the argument names, and the result field names are the same as in OpenSEO. For example, `get_keyword_metrics` returns `search_volume` and `research_keywords` returns `searchVolume`, as in OpenSEO. Nullable metrics and nested DataForSEO rows keep their JSON types.

These differences are intentional:

- No tool has a `projectId` argument, and results have no `meta` field.
- Credentials and the default market come from the server configuration or from the library caller. There is no project lookup.
- There is no OpenSEO credit accounting and no cached research. Each call uses your DataForSEO account directly.
- A successful MCP result has the result JSON in `structuredContent` and the same JSON as text in a text content block. OpenSEO returns Markdown tables in the text block.
- The tool annotations mark each tool as read-only and open-world. Read-only does not mean free: most calls spend DataForSEO balance. OpenSEO marks `get_business_reviews` and `get_business_updates` as not read-only, because they create a task. In `seo-mcp` they are read-only like the other tools, because a task changes no data that the caller can see. Like the other tools, they spend DataForSEO balance.
- `get_business_reviews` and `get_business_updates` wait up to 20 seconds, as in OpenSEO, but check the task first 4 seconds after the post. OpenSEO also checks it at once.
- A tool error contains one of these: an input correction, a short error message, or the task status message from DataForSEO. Failed authentication and rate limits return a short error message. A tool error never contains a raw HTTP response body.

`seo-mcp` has only these fifteen tools. It does not port the other OpenSEO features, such as projects and scheduled rank tracking.

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
