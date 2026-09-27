package seo

import (
	"errors"
	"testing"
)

func TestParseResearchTarget(t *testing.T) {
	for _, tt := range []struct{ name, input, scope, host, path, display, wantScope, message string }{
		{name: "default root", input: "Example.com", host: "example.com", display: "example.com", wantScope: "subdomains"},
		{name: "default folder", input: " example.com/Blog///?q=x#top ", host: "example.com", path: "/Blog", display: "example.com/Blog", wantScope: "subfolder"},
		{name: "encoded path", input: "https://www.Example.com/Docs/%7Euser/", scope: "exact_url", host: "example.com", path: "/Docs/%7Euser", display: "example.com/Docs/%7Euser", wantScope: "exact_url"},
		{name: "scope drops path", input: "blog.example.com/path", scope: "domain", host: "blog.example.com", path: "/path", display: "blog.example.com", wantScope: "domain"},
		{name: "dot segments", input: "example.com/a/../b/%2e/c//d", host: "example.com", path: "/b/c//d", display: "example.com/b/c//d", wantScope: "subfolder"},
		{name: "private suffix", input: "site.github.io", host: "site.github.io", display: "site.github.io", wantScope: "subdomains"},
		{name: "punycode suffix", input: "xn--bcher-kva.de", host: "xn--bcher-kva.de", display: "xn--bcher-kva.de", wantScope: "subdomains"},
		{name: "unicode host", input: "https://Bücher.de/Katalog", host: "xn--bcher-kva.de", path: "/Katalog", display: "xn--bcher-kva.de/Katalog", wantScope: "subfolder"},
		{name: "invalid unicode host", input: "a\u200db.com", message: "Enter a valid domain like example.com"},
		{name: "wildcard suffix", input: "example.ck", host: "example.ck", display: "example.ck", wantScope: "subdomains"},
		{name: "nested suffix", input: "example.co.za", host: "example.co.za", display: "example.co.za", wantScope: "subdomains"},
		{name: "nonterminal suffix", input: "example.za", message: "Enter a valid domain like example.com"},
		{name: "empty", input: " ", message: "Enter a domain or URL"},
		{name: "invalid tld", input: "example.por", message: "Enter a valid domain like example.com"},
		{name: "underscore", input: "my_site.com", message: "Enter a valid domain like example.com"},
		{name: "ip", input: "https://127.0.0.1/", message: "Enter a valid domain like example.com"},
		{name: "localhost", input: "localhost", message: "Enter a valid domain like example.com"},
		{name: "credentials", input: "https://user:pass@example.com/a", message: "URLs with embedded credentials are not supported"},
		{name: "invalid port", input: "https://example.com:99999/a", message: "Enter a valid domain like example.com"},
		{name: "folder without path", input: "example.com/", scope: "subfolder", message: "Add a path to use Subfolder (e.g. example.com/blog)"},
		{name: "bad scope", input: "example.com", scope: "invalid", message: "Invalid scope: invalid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseResearchTarget(tt.input, tt.scope)
			if tt.message != "" {
				var inputErr *InputError
				if !errors.As(err, &inputErr) || err.Error() != tt.message {
					t.Fatalf("got %v, want InputError %q", err, tt.message)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.hostname != tt.host || got.path != tt.path || got.display != tt.display || got.scope != tt.wantScope {
				t.Fatalf("unexpected target: %+v", got)
			}
		})
	}
}

func TestNormalizeBacklinksTarget(t *testing.T) {
	for _, tt := range []struct {
		name, input, scope, api, display, path, wantScope, message string
		subdomains                                                 bool
	}{
		{name: "domain default", input: "Example.com", api: "example.com", display: "example.com", wantScope: "subdomains", subdomains: true},
		{name: "domain", input: "https://www.Example.com/a", scope: "domain", api: "example.com", display: "example.com", wantScope: "domain"},
		{name: "folder", input: "example.com/blog/?q=1#top", api: "example.com", display: "example.com/blog", path: "/blog", wantScope: "subfolder"},
		{name: "http page", input: "http://www.Example.com:8080/Docs/", scope: "exact_url", api: "http://www.example.com/Docs", display: "http://www.example.com/Docs", wantScope: "exact_url", subdomains: true},
		{name: "legacy page", input: "Example.com", scope: "page", api: "https://example.com/", display: "https://example.com/", wantScope: "exact_url", subdomains: true},
		{name: "page query", input: "example.com/a?", scope: "exact_url", message: "Page URLs with query strings or fragments are not supported"},
		{name: "page fragment", input: "example.com/a#", scope: "page", message: "Page URLs with query strings or fragments are not supported"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeBacklinksTarget(tt.input, tt.scope)
			if tt.message != "" {
				if err == nil || err.Error() != tt.message {
					t.Fatalf("got %v, want %q", err, tt.message)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := backlinksTarget{apiTarget: tt.api, display: tt.display, scope: tt.wantScope, path: tt.path, includeSubdomains: tt.subdomains}
			if got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}
