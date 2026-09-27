// Command seo-mcp serves the SEO research tools over MCP.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/plori-ai/seo-mcp/dataforseo"
	"github.com/plori-ai/seo-mcp/seo"
	"github.com/plori-ai/seo-mcp/toolset"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "seo-mcp:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("seo-mcp", flag.ContinueOnError)
	flags.SetOutput(stderr)
	httpAddr := flags.String("http", "", "serve streamable HTTP at this address on /mcp (default: stdio)")
	locationCode := flags.Int("location-code", seo.DefaultLocationCode, "default DataForSEO location code")
	languageCode := flags.String("language-code", seo.DefaultLanguageCode, "default DataForSEO language code")
	showVersion := flags.Bool("version", false, "print version and exit")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments; use -help for usage")
	}
	if *showVersion {
		_, err := fmt.Fprintln(stdout, version)
		return err
	}
	api, err := clientFromEnv(getenv)
	if err != nil {
		return err
	}
	client := seo.New(api, seo.WithDefaultMarket(seo.Market{
		LocationCode: *locationCode,
		LanguageCode: *languageCode,
	}))
	server := newServer(toolset.New(client))
	if *httpAddr == "" {
		err := server.Run(ctx, &mcp.StdioTransport{})
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			return nil
		}
		return err
	}
	token := getenv("SEO_MCP_TOKEN")
	listener, err := listenHTTP(*httpAddr, token)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stderr, "seo-mcp: serving MCP on http://%s/mcp\n", listener.Addr())
	return serveHTTP(ctx, listener, newHTTPHandler(server, token))
}

func clientFromEnv(getenv func(string) string) (*dataforseo.Client, error) {
	if key := strings.TrimSpace(getenv("DATAFORSEO_API_KEY")); key != "" {
		return dataforseo.New(key), nil
	}
	login, password := getenv("DATAFORSEO_LOGIN"), getenv("DATAFORSEO_PASSWORD")
	if login != "" && password != "" {
		return dataforseo.NewWithLogin(login, password), nil
	}
	return nil, errors.New("set DATAFORSEO_API_KEY (base64 of login:password) or both DATAFORSEO_LOGIN and DATAFORSEO_PASSWORD")
}
