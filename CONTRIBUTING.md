# Contributing

Bug reports and pull requests are welcome through [GitHub](https://github.com/plori-ai/seo-mcp). For security problems, follow [SECURITY.md](SECURITY.md).

## Development

Use Go 1.25.0 or later and golangci-lint v2.14.0, matching CI. The CI matrix covers Go 1.25 and 1.26.

```sh
go mod download
make build
make check
```

`make build` writes `/tmp/seo-mcp` by default. Override it with `make build BIN=/path/to/seo-mcp`. Set `VERSION` to embed a version; the default is `dev`. `make check` runs `go vet ./...`, `go test -race ./...`, and `golangci-lint run ./...`. Format changed Go files with `gofmt` before submitting them.

The test suite uses synthetic responses from `httptest.Server` and requires no credentials. Examples that make paid requests have no `Output` comment, so `go test` compiles them without executing them. Do not add tests that contact DataForSEO's production service.

## Package boundaries

- `dataforseo` handles authentication, HTTP transport, retries, task envelopes, and provider errors. Keep endpoint-specific models out of it.
- `seo` handles typed research requests, market rules, validation, and result shaping. Keep MCP, storage, projects, caching, and billing out of it.
- `toolset` contains tool descriptions, raw JSON schemas, and named dispatch to `seo`. It has no MCP SDK dependency.
- `cmd/seo-mcp` adapts the toolset to MCP and handles process configuration, transports, and HTTP authentication. Only this package imports the MCP SDK.

Prefer the standard library. Explain new dependencies in the pull request. Document exported names and keep the public API small. The MCP SDK is pinned to v1.7.0; changes to that version need an explicit compatibility review.

## Tests and compatibility

Use table-driven tests and small synthetic fixtures. Assert the provider endpoint and task fields as well as the result JSON. Cover defaults, explicit market selection, invalid inputs, provider errors, empty results, and relevant fallback paths. Test cancellation where a request waits or retries.

Preserve the five OpenSEO tool names and their JSON argument and result names. Pay attention to omitted fields versus `null`, empty arrays, nesting, and numeric types. Do not add `projectId` arguments or `meta` output. Explain any intentional compatibility change in the pull request and changelog.

Never commit live provider responses, account credentials, or private hostnames. Construct fixtures yourself using the provider's documented response format. Examples should use reserved domains such as `example.com`.

## Pull requests

Describe the problem, the resulting behavior, and the checks you ran. Include regression coverage for behavior changes. Update the README for user-visible configuration changes and add an entry under `Unreleased` in [CHANGELOG.md](CHANGELOG.md). Keep unrelated refactors separate.

## Releases

Maintainers move the applicable changelog entries from `Unreleased` to a dated version section before tagging a release. Tags beginning with `v` trigger the release workflow. It runs the CI matrix before GoReleaser publishes archives for Linux, macOS, and Windows on amd64 and arm64, plus SHA-256 checksums. GoReleaser embeds the version printed by `seo-mcp -version`.
