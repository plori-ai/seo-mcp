package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func listenHTTP(address, token string) (net.Listener, error) {
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("invalid HTTP listen address: %w", err)
	}
	if token == "" && !addr.IP.IsLoopback() {
		return nil, errors.New("refusing non-loopback HTTP listener without SEO_MCP_TOKEN; tool calls spend the operator's DataForSEO balance")
	}
	// Bind the resolved IP we checked, without a second hostname lookup.
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen for HTTP: %w", err)
	}
	return listener, nil
}

func newHTTPHandler(server *mcp.Server, token string) http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless:                    true,
		JSONResponse:                 true,
		PropagateRequestCancellation: true,
	})
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	protected := http.NewCrossOriginProtection().Handler(mux)
	if token == "" {
		return protected
	}
	// Hash both values so even mismatched token lengths use a fixed-size compare.
	want := sha256.Sum256([]byte("Bearer " + token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 || len(r.Header.Values("Authorization")) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="seo-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

func serveHTTP(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       time.Minute,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	stopped := make(chan struct{})
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				_ = server.Close()
			}
		case <-stopped:
		}
	}()
	err := server.Serve(listener)
	close(stopped)
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
