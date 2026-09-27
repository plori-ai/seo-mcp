# Security policy

## Reporting a vulnerability

Report vulnerabilities privately through [GitHub's security advisory form](https://github.com/plori-ai/seo-mcp/security/advisories/new). Include the affected version, reproduction steps using synthetic data, and the impact. Do not put credentials, tokens, or live provider responses in the report. Do not open a public issue with exploit details before a fix is available.

Security fixes target the latest release. Earlier releases may require an upgrade. Until the first release, fixes target the `main` branch.

## Credentials and paid requests

DataForSEO credentials belong in the server's environment or in the embedding application's secret configuration. `DATAFORSEO_API_KEY` is base64-encoded authentication data, not encrypted data. Protect it as you would the login and password. The server does not log credentials.

Every caller uses the operator's DataForSEO account. Read-only MCP annotations describe research behavior; they do not prevent charges. The server does not implement per-user permissions, usage quotas, or spending limits.

## HTTP access

Prefer stdio for a local MCP client. HTTP mode serves `/mcp` and requires `SEO_MCP_TOKEN` for non-loopback listeners. When a token is configured, every HTTP request must carry its bearer authorization header. Loopback listeners without a token are accessible to other processes on the same host.

Use HTTPS through a reverse proxy or an encrypted tunnel for remote clients. Restrict backend network access, preserve the authorization header, and rotate the token if it is exposed. Set any additional request limits at the proxy. The server's browser-origin checks do not replace authentication or network controls.

Keywords, domains, and other research arguments are sent to DataForSEO. Returned data may contain text from external sources; applications should treat it as data rather than instructions. The library stores no research history, but clients and surrounding infrastructure may retain requests or responses.
