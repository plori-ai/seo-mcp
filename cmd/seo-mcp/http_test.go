package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const initializeRequest = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"dev"}}}`

func TestHTTPToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		headers []string
		method  string
		path    string
		want    int
	}{
		{"missing token", "sample-token", nil, "POST", "/mcp", http.StatusUnauthorized},
		{"wrong token", "sample-token", []string{"Bearer wrong"}, "POST", "/mcp", http.StatusUnauthorized},
		{"wrong scheme", "sample-token", []string{"Basic sample-token"}, "POST", "/mcp", http.StatusUnauthorized},
		{"token prefix", "sample-token", []string{"Bearer sample-token-extra"}, "POST", "/mcp", http.StatusUnauthorized},
		{"empty bearer", "sample-token", []string{"Bearer "}, "POST", "/mcp", http.StatusUnauthorized},
		{"duplicate authorization", "sample-token", []string{"Bearer sample-token", "Bearer sample-token"}, "POST", "/mcp", http.StatusUnauthorized},
		{"valid token", "sample-token", []string{"Bearer sample-token"}, "POST", "/mcp", http.StatusOK},
		{"no token configured", "", nil, "POST", "/mcp", http.StatusOK},
		{"GET needs token", "sample-token", nil, "GET", "/mcp", http.StatusUnauthorized},
		{"DELETE needs token", "sample-token", nil, "DELETE", "/mcp", http.StatusUnauthorized},
		{"OPTIONS needs token", "sample-token", nil, "OPTIONS", "/mcp", http.StatusUnauthorized},
		{"other path needs token", "sample-token", nil, "GET", "/", http.StatusUnauthorized},
		{"authenticated other path", "sample-token", []string{"Bearer sample-token"}, "GET", "/", http.StatusNotFound},
		{"stateless GET", "sample-token", []string{"Bearer sample-token"}, "GET", "/mcp", http.StatusMethodNotAllowed},
		{"stateless DELETE", "sample-token", []string{"Bearer sample-token"}, "DELETE", "/mcp", http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "dev"}, nil)
			fake := httptest.NewServer(newHTTPHandler(server, tt.token))
			defer fake.Close()
			req, err := http.NewRequestWithContext(t.Context(), tt.method, fake.URL+tt.path, strings.NewReader(initializeRequest))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			for _, header := range tt.headers {
				req.Header.Add("Authorization", header)
			}
			response, err := fake.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tt.want {
				t.Fatalf("status = %d, want %d: %s", response.StatusCode, tt.want, body)
			}
			if tt.want == http.StatusOK {
				var rpc struct {
					Result *mcp.InitializeResult `json:"result"`
				}
				if err := json.Unmarshal(body, &rpc); err != nil || rpc.Result == nil || rpc.Result.ServerInfo == nil || rpc.Result.ServerInfo.Name != "test" {
					t.Errorf("initialize response = %s, error = %v", body, err)
				}
			}
			if tt.want == http.StatusUnauthorized && response.Header.Get("WWW-Authenticate") == "" {
				t.Error("missing bearer challenge")
			}
		})
	}
}

func TestHTTPCrossOrigin(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "dev"}, nil)
	handler := newHTTPHandler(server, "")
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", strings.NewReader(initializeRequest))
	req.Header.Set("Origin", "https://untrusted.example")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-origin POST status = %d, want 403", recorder.Code)
	}
}

func TestListenHTTP(t *testing.T) {
	tests := []struct {
		name    string
		address string
		token   string
		wantErr string
	}{
		{"IPv4 loopback", "127.0.0.1:0", "", ""},
		{"localhost", "localhost:0", "", ""},
		{"wildcard IPv4", "0.0.0.0:0", "", "SEO_MCP_TOKEN"},
		{"wildcard IPv6", "[::]:0", "", "SEO_MCP_TOKEN"},
		{"empty host", ":0", "", "SEO_MCP_TOKEN"},
		{"external IP", "192.0.2.1:0", "", "SEO_MCP_TOKEN"},
		{"authenticated wildcard", "0.0.0.0:0", "sample-token", ""},
		{"invalid address", "127.0.0.1", "", "invalid HTTP listen address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listener, err := listenHTTP(tt.address, tt.token)
			if listener != nil {
				defer func() { _ = listener.Close() }()
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServeHTTPShutdown(t *testing.T) {
	listener, err := listenHTTP("127.0.0.1:0", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serveHTTP(ctx, listener, http.NotFoundHandler()) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("HTTP server did not shut down")
	}
}
