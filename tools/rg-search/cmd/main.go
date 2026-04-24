package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/op-agent/opagent-protocol/go-sdk/op"
	"github.com/spf13/cobra"
)

type searchInput struct {
	Root       string   `json:"root,omitempty" jsonschema:"Absolute or relative directory root to search"`
	Query      string   `json:"query" jsonschema:"Search pattern"`
	Regex      bool     `json:"regex,omitempty" jsonschema:"Treat query as regex"`
	MatchCase  bool     `json:"matchCase,omitempty" jsonschema:"Case-sensitive search"`
	WholeWord  bool     `json:"wholeWord,omitempty" jsonschema:"Match whole words only"`
	Includes   []string `json:"includes,omitempty" jsonschema:"Optional include globs"`
	Excludes   []string `json:"excludes,omitempty" jsonschema:"Optional exclude globs"`
	MaxFiles   int      `json:"maxFiles,omitempty" jsonschema:"Optional max files in the result"`
	MaxMatches int      `json:"maxMatches,omitempty" jsonschema:"Optional max matches in the result"`
}

var rootCmd = &cobra.Command{
	Use:   "rg-search",
	Short: "system-level ripgrep helper",
	RunE: func(_ *cobra.Command, _ []string) error {
		return run()
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "rg-search failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer stop()

	server := op.NewServer(&op.Implementation{Name: "rg-search", Version: "v1.0.0"}, nil)
	op.AddTool(server, &op.Tool{
		Name:        "rg_search",
		Description: "Search files with the bundled ripgrep binary.",
	}, handleSearch)
	if err := server.Run(ctx, &op.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func handleSearch(ctx context.Context, req *op.CallToolRequest, input searchInput) (*op.CallToolResult, Result, error) {
	root, err := resolveRoot(req, input.Root)
	if err != nil {
		return nil, Result{}, err
	}
	exePath, err := os.Executable()
	if err != nil {
		return nil, Result{}, err
	}
	binaryPath := filepath.Join(filepath.Dir(exePath), "rg")
	result, err := Search(ctx, binaryPath, Query{
		Root:       root,
		Pattern:    strings.TrimSpace(input.Query),
		Regex:      input.Regex,
		MatchCase:  input.MatchCase,
		WholeWord:  input.WholeWord,
		Includes:   append([]string(nil), input.Includes...),
		Excludes:   append([]string(nil), input.Excludes...),
		MaxFiles:   input.MaxFiles,
		MaxMatches: input.MaxMatches,
	})
	if err != nil {
		return nil, Result{}, err
	}
	return nil, result, nil
}

func resolveRoot(req *op.CallToolRequest, explicit string) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		return ResolveAbsolutePath("", explicit)
	}
	if req != nil && req.Params.Meta != nil {
		if raw := strings.TrimSpace(fmt.Sprint(req.Params.Meta["workdir"])); raw != "" {
			return ResolveAbsolutePath("", raw)
		}
	}
	return os.Getwd()
}
