# Security policy

## Report a vulnerability

Report vulnerabilities privately with the [GitHub security advisory form](https://github.com/plori-ai/seo-mcp/security/advisories/new). Include the affected version, the steps to reproduce the problem with synthetic data, and the impact. Do not put credentials, tokens, or live provider responses in the report. Do not open a public issue with exploit details before a fix is available.

We release security fixes for the latest version only. To get a fix, upgrade to the latest release.

## Credentials and paid requests

Keep the DataForSEO credentials in the environment of the server, or in the secret configuration of the application that embeds the library. `DATAFORSEO_API_KEY` is base64-encoded, not encrypted. Protect it as you protect the login and password. The server does not log credentials.

All callers use the DataForSEO account of the operator. The read-only MCP annotations describe what the tools do. They do not prevent charges. The server has no per-user permissions, no usage quotas, and no spending limits.

## HTTP access

For a local MCP client, use stdio if you can. HTTP mode serves `/mcp` and needs `SEO_MCP_TOKEN` for a non-loopback address. If you set a token, each HTTP request must have the matching bearer `Authorization` header. A loopback listener without a token accepts requests from all processes on the same host.

For remote clients, use HTTPS through a reverse proxy or a different encrypted connection. Block direct network access to the backend listener, and make the proxy forward the `Authorization` header. If the token becomes known to others, replace it. Set request limits at the proxy, because the server has none. The server rejects cross-origin browser requests, but this check does not replace authentication or network controls.

## Research data

The server sends keywords, domains, and the other research arguments to DataForSEO. The returned data can contain text from external websites. Applications must treat this text as data, not as instructions. The library and the server keep no research history. MCP clients and other infrastructure around the server can keep requests or responses.
