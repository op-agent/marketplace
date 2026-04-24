package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/op-agent/opagent-dev/packages/opagent-protocol-sdk-go/op"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "research-tools failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	workdir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve workdir: %w", err)
	}

	cfg, err := loadToolConfig(workdir)
	if err != nil {
		return err
	}

	service := newToolService(cfg, workdir)
	server := op.NewServer(&op.Implementation{Name: "research-tools", Version: "v1.0.0"}, nil)
	op.AddTool(server, &op.Tool{
		Name:        "web_search",
		Description: "Search the web with Tavily and return normalized research results.",
	}, service.HandleWebSearch)
	op.AddTool(server, &op.Tool{
		Name:        "web_fetch",
		Description: "Fetch and normalize a webpage using Jina Reader first and direct HTTP fallback second.",
	}, service.HandleWebFetch)
	op.AddTool(server, &op.Tool{
		Name:        "browser_search",
		Description: "Search the web through agent-browser and normalize the result snapshot.",
	}, service.HandleBrowserSearch)
	op.AddTool(server, &op.Tool{
		Name:        "browser_fetch",
		Description: "Open a page through agent-browser and return normalized page text from the browser snapshot.",
	}, service.HandleBrowserFetch)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer stop()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := server.Run(ctx, &op.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
