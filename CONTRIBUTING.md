# Contributing

You can send bug reports and pull requests through [GitHub](https://github.com/plori-ai/seo-mcp). For security problems, follow [SECURITY.md](SECURITY.md).

## Development

Use Go 1.25.0 or later and golangci-lint v2.14.0. CI uses the same golangci-lint version and runs the checks on Go 1.25 and Go 1.26.

```sh
go mod download
make build
make check
```

`make build` writes the binary to `/tmp/seo-mcp`. To write it to a different path, run `make build BIN=/path/to/seo-mcp`. To put a version in the binary, set `VERSION`. The default version is `dev`.

`make check` runs `go vet ./...`, `go test -race ./...`, and `golangci-lint run ./...`. You can also run each check alone with `make vet`, `make test`, or `make lint`.

Format your Go changes with `gofmt` before you send them. `make fmt` runs `gofmt -w` on all Go files in the four packages.

The tests use synthetic responses from `httptest.Server` and need no credentials. The examples that make paid requests have no `Output` comment, so `go test` compiles them but does not run them. Do not add tests that send requests to the DataForSEO production API.

## Package boundaries

- `dataforseo` does authentication, HTTP transport, retries, task envelopes, and provider errors. Do not put endpoint-specific models in it.
- `seo` does typed research requests, market rules, validation, and result shaping. Do not put MCP, storage, projects, caching, or billing in it.
- `toolset` has the tool descriptions, the raw JSON schemas, and the dispatch by tool name to `seo`. It does not depend on an MCP SDK.
- `cmd/seo-mcp` connects the toolset to MCP. It also does process configuration, transports, and HTTP authentication. It is the only package that imports the MCP SDK.

Use the standard library when you can. If you add a dependency, explain why in the pull request. Write documentation for each exported name, and keep the public API small. `go.mod` pins the MCP SDK at v1.7.0. A change to that version needs a compatibility review.

## Tests and compatibility

Use table-driven tests and small synthetic fixtures. In each test, check the DataForSEO endpoint and the task fields, and also the result JSON. Cover defaults, explicit market selection, invalid inputs, provider errors, empty results, and the fallback paths that apply. If a request waits or retries, test cancellation.

Keep the five OpenSEO tool names and their JSON argument and result names. Check omitted fields against `null`, empty arrays, nesting, and numeric types. Do not add `projectId` arguments or `meta` output. If you make an intentional compatibility change, explain it in the pull request and in the changelog.

Do not commit live provider responses, account credentials, or private hostnames. Make fixtures yourself from the documented DataForSEO response format. In examples, use reserved domains such as `example.com`.

## Pull requests

In the pull request, describe the problem, the new behavior, and the checks you ran. Add regression tests for behavior changes. If you change user-visible configuration, update the README. Add an entry under `Unreleased` in [CHANGELOG.md](CHANGELOG.md). Put unrelated refactors in a separate pull request.

## Releases

Maintainers make a release with these steps:

1. In [CHANGELOG.md](CHANGELOG.md), move the entries from `Unreleased` to a new section with the version and the date. Update the links at the end of the file.
2. Push a tag that starts with `v`, for example `v0.1.0`.
3. The Release workflow runs the CI checks on Go 1.25 and Go 1.26. If they pass, GoReleaser builds the release.

GoReleaser publishes archives for Linux, macOS, and Windows, on amd64 and arm64, and a `checksums.txt` file with SHA-256 checksums. It sets the version that `seo-mcp -version` prints. A tag with a prerelease suffix, such as `v0.2.0-rc.1`, makes a GitHub prerelease.
