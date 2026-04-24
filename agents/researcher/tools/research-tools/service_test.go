package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/op-agent/opagent-protocol/go-sdk/op"
)

type mockRunner struct {
	outputs []string
	err     error
	calls   []string
}

func (m *mockRunner) Run(_ context.Context, name string, args []string, dir string) (string, error) {
	m.calls = append(m.calls, name+" "+strings.Join(args, " ")+" @ "+dir)
	if m.err != nil {
		return "", m.err
	}
	if len(m.outputs) == 0 {
		return "", nil
	}
	next := m.outputs[0]
	m.outputs = m.outputs[1:]
	return next, nil
}

func TestLoadToolConfigResolvesSecrets(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	secretsPath := filepath.Join(homeDir, ".opagent", "configs", "secrets.env")
	if err := os.MkdirAll(filepath.Dir(secretsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}
	if err := os.WriteFile(secretsPath, []byte("tavily_api_key=test-key\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(secrets): %v", err)
	}

	workdir := t.TempDir()
	configBody := `{"tavily_api_key":"@tavily_api_key","browser_command":"agent-browser"}`
	if err := os.WriteFile(filepath.Join(workdir, "config.json"), []byte(configBody), 0o644); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	cfg, err := loadToolConfig(workdir)
	if err != nil {
		t.Fatalf("loadToolConfig(): %v", err)
	}
	if cfg.TavilyAPIKey != "test-key" {
		t.Fatalf("cfg.TavilyAPIKey = %q, want %q", cfg.TavilyAPIKey, "test-key")
	}
}

func TestLoadToolConfigAllowsMissingOptionalSecrets(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	workdir := t.TempDir()
	configBody := `{"tavily_api_key":"@missing_tavily_key","jina_api_key":"@missing_jina_key","browser_command":"agent-browser"}`
	if err := os.WriteFile(filepath.Join(workdir, "config.json"), []byte(configBody), 0o644); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	cfg, err := loadToolConfig(workdir)
	if err != nil {
		t.Fatalf("loadToolConfig(): %v", err)
	}
	if cfg.TavilyAPIKey != "" {
		t.Fatalf("cfg.TavilyAPIKey = %q, want empty", cfg.TavilyAPIKey)
	}
	if cfg.JinaAPIKey != "" {
		t.Fatalf("cfg.JinaAPIKey = %q, want empty", cfg.JinaAPIKey)
	}
}

func TestWebSearchNormalizesTavilyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["query"] != "opagent researcher" {
			t.Fatalf("query = %v", payload["query"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"title":          "Researcher",
					"url":            "https://example.com/researcher",
					"content":        "A dedicated research workflow.",
					"published_date": "2026-03-26",
				},
			},
		})
	}))
	defer server.Close()

	svc := newToolService(toolConfig{
		TavilyBaseURL: server.URL,
		TavilyAPIKey:  "token",
		UserAgent:     "tester",
		MaxFetchChars: 1000,
	}, t.TempDir())

	results, err := svc.webSearch(context.Background(), "opagent researcher", 3, nil, nil)
	if err != nil {
		t.Fatalf("webSearch(): %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].URL != "https://example.com/researcher" {
		t.Fatalf("url = %q", results[0].URL)
	}
	if results[0].Source != "tavily" {
		t.Fatalf("source = %q", results[0].Source)
	}
}

func TestWebFetchFallsBackToDirectHTML(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Example Page</title></head><body><main><h1>Headline</h1><p>Hello world from HTML.</p></main></body></html>`))
	}))
	defer page.Close()

	svc := newToolService(toolConfig{
		JinaReaderPrefix: "http://127.0.0.1:1/http://",
		UserAgent:        "tester",
		MaxFetchChars:    1000,
	}, t.TempDir())

	doc, err := svc.webFetch(context.Background(), page.URL, nil)
	if err != nil {
		t.Fatalf("webFetch(): %v", err)
	}
	if doc.Title != "Example Page" {
		t.Fatalf("doc.Title = %q, want %q", doc.Title, "Example Page")
	}
	if !strings.Contains(doc.Content, "Hello world from HTML.") {
		t.Fatalf("doc.Content = %q, want HTML body text", doc.Content)
	}
	if doc.Source != "http" {
		t.Fatalf("doc.Source = %q, want %q", doc.Source, "http")
	}
}

func TestBrowserSearchParsesSnapshotResults(t *testing.T) {
	runner := &mockRunner{
		outputs: []string{
			"",
			`@e1 OpenAI https://openai.com
OpenAI research and products
@e2 Tavily https://tavily.com
Search tuned for agents`,
		},
	}
	svc := newToolService(toolConfig{
		BrowserCommand:           "agent-browser",
		BrowserSearchURLTemplate: "https://duckduckgo.com/?q=%s",
		BrowserOpenArgs:          []string{"open"},
		BrowserSnapshotArgs:      []string{"snapshot", "-i"},
		MaxFetchChars:            1000,
	}, t.TempDir())
	svc.runner = runner

	results, snapshot, err := svc.browserSearch(context.Background(), "ai search", 5, "")
	if err != nil {
		t.Fatalf("browserSearch(): %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].URL != "https://openai.com" {
		t.Fatalf("first url = %q", results[0].URL)
	}
	if !strings.Contains(snapshot, "OpenAI") {
		t.Fatalf("snapshot = %q, want content", snapshot)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("len(runner.calls) = %d, want 2", len(runner.calls))
	}
}

func TestBrowserFetchNormalizesSnapshotText(t *testing.T) {
	runner := &mockRunner{
		outputs: []string{
			"",
			"@e1 Example heading\nThis is browser extracted text.",
		},
	}
	svc := newToolService(toolConfig{
		BrowserCommand:      "agent-browser",
		BrowserOpenArgs:     []string{"open"},
		BrowserSnapshotArgs: []string{"snapshot", "-i"},
		MaxFetchChars:       1000,
	}, t.TempDir())
	svc.runner = runner

	doc, _, err := svc.browserFetch(context.Background(), "https://example.com", "networkidle", nil)
	if err != nil {
		t.Fatalf("browserFetch(): %v", err)
	}
	if doc.Source != "agent-browser" {
		t.Fatalf("doc.Source = %q", doc.Source)
	}
	if !strings.Contains(doc.Content, "This is browser extracted text.") {
		t.Fatalf("doc.Content = %q", doc.Content)
	}
}

func TestHandleWebSearchFallsBackToBrowserWithoutAPIKey(t *testing.T) {
	runner := &mockRunner{
		outputs: []string{
			"",
			"@e1 Result https://example.com\nExample snippet",
		},
	}
	svc := newToolService(toolConfig{
		BrowserCommand:           "agent-browser",
		BrowserSearchURLTemplate: "https://duckduckgo.com/?q=%s",
		BrowserOpenArgs:          []string{"open"},
		BrowserSnapshotArgs:      []string{"snapshot", "-i"},
		MaxFetchChars:            1000,
	}, t.TempDir())
	svc.runner = runner

	result, _, err := svc.HandleWebSearch(context.Background(), &op.CallToolRequest{}, webSearchInput{
		Query: "fallback search",
	})
	if err != nil {
		t.Fatalf("HandleWebSearch(): %v", err)
	}
	text := result.Content[0].(*op.TextContent).Text
	if !strings.Contains(text, "browser fallback") {
		t.Fatalf("text = %q, want browser fallback marker", text)
	}
}
