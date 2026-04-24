package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/op-agent/opagent-dev/packages/opagent-protocol-sdk-go/op"
	"golang.org/x/net/html"
)

const (
	defaultSearchResults = 5
	defaultFetchTimeout  = 20 * time.Second
	defaultBrowserTime   = 30 * time.Second
	maxFetchBodyBytes    = 1 << 20
)

type commandRunner interface {
	Run(ctx context.Context, name string, args []string, dir string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args []string, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(stderr.String())
		if text == "" {
			text = strings.TrimSpace(stdout.String())
		}
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), text)
	}
	if text := strings.TrimSpace(stdout.String()); text != "" {
		return text, nil
	}
	return strings.TrimSpace(stderr.String()), nil
}

type toolService struct {
	cfg        toolConfig
	workdir    string
	httpClient *http.Client
	runner     commandRunner
}

type webSearchInput struct {
	Query       string   `json:"query" jsonschema:"Search query"`
	MaxResults  int      `json:"max_results,omitempty" jsonschema:"Maximum number of results to return"`
	Domains     []string `json:"domains,omitempty" jsonschema:"Optional domain allowlist"`
	RecencyDays *int     `json:"recency_days,omitempty" jsonschema:"Optional recency window in days"`
}

type webFetchInput struct {
	URL            string `json:"url" jsonschema:"Absolute URL to fetch"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty" jsonschema:"Fetch timeout in seconds"`
}

type browserSearchInput struct {
	Query      string `json:"query" jsonschema:"Search query"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"Maximum number of results to return"`
	Locale     string `json:"locale,omitempty" jsonschema:"Optional locale hint"`
}

type browserFetchInput struct {
	URL            string `json:"url" jsonschema:"Absolute URL to fetch through agent-browser"`
	WaitUntil      string `json:"wait_until,omitempty" jsonschema:"Optional browser wait hint"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty" jsonschema:"Browser timeout in seconds"`
}

type searchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Snippet     string `json:"snippet,omitempty"`
	Source      string `json:"source"`
	PublishedAt string `json:"published_at,omitempty"`
}

type fetchedDocument struct {
	URL      string `json:"url"`
	FinalURL string `json:"final_url,omitempty"`
	Title    string `json:"title,omitempty"`
	Content  string `json:"content"`
	Source   string `json:"source"`
}

func newToolService(cfg toolConfig, workdir string) *toolService {
	return &toolService{
		cfg:        cfg,
		workdir:    workdir,
		httpClient: &http.Client{},
		runner:     execRunner{},
	}
}

func (s *toolService) HandleWebSearch(ctx context.Context, req *op.CallToolRequest, input webSearchInput) (*op.CallToolResult, any, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, nil, fmt.Errorf("query is required")
	}

	if strings.TrimSpace(s.cfg.TavilyAPIKey) == "" {
		results, snapshot, err := s.browserSearch(ctx, query, input.MaxResults, "")
		if err != nil {
			return nil, nil, fmt.Errorf("web_search unavailable and browser fallback failed: %w", err)
		}
		text := formatSearchResults("web_search (browser fallback)", query, results)
		return toolResult(req, text, map[string]any{
			"query":    query,
			"results":  results,
			"snapshot": truncateText(snapshot, s.cfg.MaxFetchChars),
			"source":   "agent-browser",
		}), nil, nil
	}

	results, err := s.webSearch(ctx, query, input.MaxResults, input.Domains, input.RecencyDays)
	if err != nil || len(results) == 0 {
		fallbackResults, snapshot, fallbackErr := s.browserSearch(ctx, query, input.MaxResults, "")
		if fallbackErr != nil {
			if err != nil {
				return nil, nil, err
			}
			return nil, nil, fallbackErr
		}
		text := formatSearchResults("web_search (browser fallback)", query, fallbackResults)
		return toolResult(req, text, map[string]any{
			"query":    query,
			"results":  fallbackResults,
			"snapshot": truncateText(snapshot, s.cfg.MaxFetchChars),
			"source":   "agent-browser",
		}), nil, nil
	}
	text := formatSearchResults("web_search", query, results)
	return toolResult(req, text, map[string]any{
		"query":   query,
		"results": results,
		"source":  "tavily",
	}), nil, nil
}

func (s *toolService) HandleWebFetch(ctx context.Context, req *op.CallToolRequest, input webFetchInput) (*op.CallToolResult, any, error) {
	targetURL := strings.TrimSpace(input.URL)
	if targetURL == "" {
		return nil, nil, fmt.Errorf("url is required")
	}

	doc, err := s.webFetch(ctx, targetURL, input.TimeoutSeconds)
	if err != nil || looksLikeWeakContent(doc.Content) {
		fallbackDoc, snapshot, fallbackErr := s.browserFetch(ctx, targetURL, "", input.TimeoutSeconds)
		if fallbackErr != nil {
			if err != nil {
				return nil, nil, err
			}
			return nil, nil, fallbackErr
		}
		text := formatFetchedDocument("web_fetch (browser fallback)", fallbackDoc)
		return toolResult(req, text, map[string]any{
			"document": fallbackDoc,
			"snapshot": truncateText(snapshot, s.cfg.MaxFetchChars),
			"source":   "agent-browser",
		}), nil, nil
	}
	text := formatFetchedDocument("web_fetch", doc)
	return toolResult(req, text, doc), nil, nil
}

func (s *toolService) HandleBrowserSearch(ctx context.Context, req *op.CallToolRequest, input browserSearchInput) (*op.CallToolResult, any, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, nil, fmt.Errorf("query is required")
	}

	results, snapshot, err := s.browserSearch(ctx, query, input.MaxResults, input.Locale)
	if err != nil {
		return nil, nil, err
	}
	text := formatSearchResults("browser_search", query, results)
	if strings.TrimSpace(snapshot) != "" && len(results) == 0 {
		text += "\n\nRaw snapshot:\n" + truncateText(snapshot, s.cfg.MaxFetchChars)
	}
	return toolResult(req, text, map[string]any{
		"query":    query,
		"results":  results,
		"snapshot": truncateText(snapshot, s.cfg.MaxFetchChars),
		"source":   "agent-browser",
	}), nil, nil
}

func (s *toolService) HandleBrowserFetch(ctx context.Context, req *op.CallToolRequest, input browserFetchInput) (*op.CallToolResult, any, error) {
	targetURL := strings.TrimSpace(input.URL)
	if targetURL == "" {
		return nil, nil, fmt.Errorf("url is required")
	}

	doc, snapshot, err := s.browserFetch(ctx, targetURL, input.WaitUntil, input.TimeoutSeconds)
	if err != nil {
		return nil, nil, err
	}
	text := formatFetchedDocument("browser_fetch", doc)
	if strings.TrimSpace(snapshot) != "" && doc.Content == "" {
		text += "\n\nRaw snapshot:\n" + truncateText(snapshot, s.cfg.MaxFetchChars)
	}
	return toolResult(req, text, map[string]any{
		"document": doc,
		"snapshot": truncateText(snapshot, s.cfg.MaxFetchChars),
	}), nil, nil
}

func (s *toolService) webSearch(ctx context.Context, query string, maxResults int, domains []string, recencyDays *int) ([]searchResult, error) {
	maxResults = normalizeMaxResults(maxResults)

	body := map[string]any{
		"api_key":     s.cfg.TavilyAPIKey,
		"query":       query,
		"max_results": maxResults,
	}
	if len(domains) > 0 {
		body["include_domains"] = domains
	}
	if recencyDays != nil && *recencyDays > 0 {
		body["days"] = *recencyDays
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal tavily request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.TavilyBaseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build tavily request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", s.cfg.UserAgent)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily request failed: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("read tavily response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tavily request failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}

	var result struct {
		Results []struct {
			Title         string `json:"title"`
			URL           string `json:"url"`
			Content       string `json:"content"`
			PublishedDate string `json:"published_date"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return nil, fmt.Errorf("decode tavily response: %w", err)
	}

	out := make([]searchResult, 0, len(result.Results))
	for _, item := range result.Results {
		out = append(out, searchResult{
			Title:       strings.TrimSpace(item.Title),
			URL:         strings.TrimSpace(item.URL),
			Snippet:     truncateText(strings.TrimSpace(item.Content), 400),
			Source:      "tavily",
			PublishedAt: strings.TrimSpace(item.PublishedDate),
		})
	}
	return out, nil
}

func (s *toolService) webFetch(ctx context.Context, targetURL string, timeoutSeconds *int) (fetchedDocument, error) {
	timeout := defaultFetchTimeout
	if timeoutSeconds != nil && *timeoutSeconds > 0 {
		timeout = time.Duration(*timeoutSeconds) * time.Second
	}
	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if doc, err := s.fetchWithJina(fetchCtx, targetURL); err == nil && !looksLikeWeakContent(doc.Content) {
		return doc, nil
	}
	return s.fetchDirect(fetchCtx, targetURL)
}

func (s *toolService) fetchWithJina(ctx context.Context, targetURL string) (fetchedDocument, error) {
	prefix := strings.TrimSpace(s.cfg.JinaReaderPrefix)
	if prefix == "" {
		return fetchedDocument{}, fmt.Errorf("jina reader is not configured")
	}

	target := strings.TrimPrefix(targetURL, "https://")
	target = strings.TrimPrefix(target, "http://")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, prefix+target, nil)
	if err != nil {
		return fetchedDocument{}, fmt.Errorf("build jina request: %w", err)
	}
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	if strings.TrimSpace(s.cfg.JinaAPIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.JinaAPIKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fetchedDocument{}, fmt.Errorf("jina request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBodyBytes))
	if err != nil {
		return fetchedDocument{}, fmt.Errorf("read jina response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fetchedDocument{}, fmt.Errorf("jina request failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	content := truncateText(strings.TrimSpace(string(body)), s.cfg.MaxFetchChars)
	return fetchedDocument{
		URL:      targetURL,
		FinalURL: targetURL,
		Title:    extractLeadingTitle(content),
		Content:  content,
		Source:   "jina",
	}, nil
}

func (s *toolService) fetchDirect(ctx context.Context, targetURL string) (fetchedDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return fetchedDocument{}, fmt.Errorf("build fetch request: %w", err)
	}
	req.Header.Set("User-Agent", s.cfg.UserAgent)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fetchedDocument{}, fmt.Errorf("direct fetch failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBodyBytes))
	if err != nil {
		return fetchedDocument{}, fmt.Errorf("read direct fetch body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fetchedDocument{}, fmt.Errorf("direct fetch failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	title := ""
	content := ""
	if strings.Contains(contentType, "html") || bytes.Contains(bytes.ToLower(body), []byte("<html")) {
		title, content = extractReadableText(body, s.cfg.MaxFetchChars)
	} else {
		content = truncateText(strings.TrimSpace(string(body)), s.cfg.MaxFetchChars)
	}
	if content == "" {
		content = truncateText(strings.TrimSpace(string(body)), s.cfg.MaxFetchChars)
	}

	finalURL := targetURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return fetchedDocument{
		URL:      targetURL,
		FinalURL: finalURL,
		Title:    title,
		Content:  content,
		Source:   "http",
	}, nil
}

func (s *toolService) browserSearch(ctx context.Context, query string, maxResults int, locale string) ([]searchResult, string, error) {
	maxResults = normalizeMaxResults(maxResults)
	searchURL := fmt.Sprintf(s.cfg.BrowserSearchURLTemplate, url.QueryEscape(query))
	if strings.TrimSpace(locale) != "" {
		separator := "&"
		if !strings.Contains(searchURL, "?") {
			separator = "?"
		}
		searchURL += separator + "locale=" + url.QueryEscape(locale)
	}

	snapshot, err := s.runBrowserSnapshot(ctx, searchURL, defaultBrowserTimeout(nil))
	if err != nil {
		return nil, "", err
	}

	results := parseBrowserSearchResults(snapshot, maxResults)
	if len(results) == 0 && strings.TrimSpace(snapshot) != "" {
		results = []searchResult{{
			Title:   "Browser search snapshot",
			URL:     searchURL,
			Snippet: truncateText(snapshot, 400),
			Source:  "agent-browser",
		}}
	}
	return results, snapshot, nil
}

func (s *toolService) browserFetch(ctx context.Context, targetURL string, waitUntil string, timeoutSeconds *int) (fetchedDocument, string, error) {
	timeout := defaultBrowserTimeout(timeoutSeconds)
	snapshot, err := s.runBrowserSnapshot(ctx, targetURL, timeout)
	if err != nil {
		return fetchedDocument{}, "", err
	}

	content := normalizeBrowserText(snapshot, s.cfg.MaxFetchChars)
	if strings.TrimSpace(waitUntil) != "" {
		content = fmt.Sprintf("wait_until=%s\n\n%s", strings.TrimSpace(waitUntil), content)
	}
	return fetchedDocument{
		URL:      targetURL,
		FinalURL: targetURL,
		Title:    extractLeadingTitle(content),
		Content:  content,
		Source:   "agent-browser",
	}, snapshot, nil
}

func (s *toolService) runBrowserSnapshot(ctx context.Context, targetURL string, timeout time.Duration) (string, error) {
	if strings.TrimSpace(s.cfg.BrowserCommand) == "" {
		return "", fmt.Errorf("browser_command is not configured")
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	openArgs := append([]string(nil), s.cfg.BrowserOpenArgs...)
	openArgs = append(openArgs, targetURL)
	if _, err := s.runner.Run(runCtx, s.cfg.BrowserCommand, openArgs, s.workdir); err != nil {
		return "", err
	}

	snapshotArgs := append([]string(nil), s.cfg.BrowserSnapshotArgs...)
	return s.runner.Run(runCtx, s.cfg.BrowserCommand, snapshotArgs, s.workdir)
}

func toolResult(req *op.CallToolRequest, text string, structured any) *op.CallToolResult {
	result := &op.CallToolResult{
		Content: []op.Content{&op.TextContent{Text: text}},
	}
	if req != nil && req.Params != nil && req.Params.Meta != nil {
		result.Meta = req.Params.Meta.Clone()
	}
	if structured != nil {
		result.StructuredContent = structured
	}
	return result
}

func normalizeMaxResults(v int) int {
	if v <= 0 {
		return defaultSearchResults
	}
	if v > 10 {
		return 10
	}
	return v
}

func defaultBrowserTimeout(timeoutSeconds *int) time.Duration {
	if timeoutSeconds != nil && *timeoutSeconds > 0 {
		return time.Duration(*timeoutSeconds) * time.Second
	}
	return defaultBrowserTime
}

func formatSearchResults(toolName string, query string, results []searchResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s results for %q\n", toolName, query)
	if len(results) == 0 {
		b.WriteString("No results found.")
		return b.String()
	}
	for i, item := range results {
		fmt.Fprintf(&b, "%d. %s\n", i+1, fallbackString(item.Title, item.URL))
		if item.URL != "" {
			fmt.Fprintf(&b, "   URL: %s\n", item.URL)
		}
		if item.Snippet != "" {
			fmt.Fprintf(&b, "   Snippet: %s\n", item.Snippet)
		}
		if item.PublishedAt != "" {
			fmt.Fprintf(&b, "   Published: %s\n", item.PublishedAt)
		}
	}
	return strings.TrimSpace(b.String())
}

func formatFetchedDocument(toolName string, doc fetchedDocument) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s fetched %s\n", toolName, doc.URL)
	if doc.FinalURL != "" && doc.FinalURL != doc.URL {
		fmt.Fprintf(&b, "Final URL: %s\n", doc.FinalURL)
	}
	if doc.Title != "" {
		fmt.Fprintf(&b, "Title: %s\n", doc.Title)
	}
	fmt.Fprintf(&b, "Source: %s\n\n", doc.Source)
	b.WriteString(strings.TrimSpace(doc.Content))
	return strings.TrimSpace(b.String())
}

func fallbackString(primary string, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return strings.TrimSpace(primary)
	}
	return strings.TrimSpace(fallback)
}

func extractLeadingTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return truncateText(line, 200)
		}
	}
	return ""
}

func looksLikeWeakContent(content string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(content))
	if len(trimmed) < 200 {
		return true
	}
	weakSignals := []string{
		"enable javascript",
		"just a moment",
		"access denied",
		"please verify you are human",
		"captcha",
	}
	for _, signal := range weakSignals {
		if strings.Contains(trimmed, signal) {
			return true
		}
	}
	return false
}

func extractReadableText(body []byte, limit int) (string, string) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return "", truncateText(strings.TrimSpace(string(body)), limit)
	}

	title := ""
	var textParts []string
	var walk func(*html.Node, bool)
	walk = func(node *html.Node, skip bool) {
		if node == nil {
			return
		}
		if node.Type == html.ElementNode {
			name := strings.ToLower(node.Data)
			if name == "script" || name == "style" || name == "noscript" {
				skip = true
			}
			if name == "title" && node.FirstChild != nil && title == "" {
				title = strings.TrimSpace(node.FirstChild.Data)
			}
		}
		if !skip && node.Type == html.TextNode {
			text := normalizeWhitespace(node.Data)
			if text != "" {
				textParts = append(textParts, text)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child, skip)
		}
	}
	walk(doc, false)
	return title, truncateText(strings.Join(textParts, "\n"), limit)
}

func normalizeBrowserText(snapshot string, limit int) string {
	lines := strings.Split(snapshot, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		cleaned := normalizeWhitespace(stripBrowserRefs(line))
		if cleaned == "" {
			continue
		}
		out = append(out, cleaned)
	}
	return truncateText(strings.Join(out, "\n"), limit)
}

var (
	browserRefPattern = regexp.MustCompile(`@e\d+`)
	urlPattern        = regexp.MustCompile(`https?://[^\s\)\]">]+`)
)

func parseBrowserSearchResults(snapshot string, maxResults int) []searchResult {
	lines := strings.Split(snapshot, "\n")
	results := make([]searchResult, 0, maxResults)
	seen := make(map[string]struct{})

	for i, line := range lines {
		urls := urlPattern.FindAllString(line, -1)
		if len(urls) == 0 {
			continue
		}
		for _, rawURL := range urls {
			if _, exists := seen[rawURL]; exists {
				continue
			}
			seen[rawURL] = struct{}{}

			title := strings.TrimSpace(stripBrowserRefs(urlPattern.ReplaceAllString(line, "")))
			if title == "" && i > 0 {
				title = normalizeWhitespace(stripBrowserRefs(lines[i-1]))
			}
			snippet := ""
			if i+1 < len(lines) {
				next := normalizeWhitespace(stripBrowserRefs(lines[i+1]))
				if next != "" && !urlPattern.MatchString(next) {
					snippet = next
				}
			}

			results = append(results, searchResult{
				Title:   truncateText(title, 200),
				URL:     rawURL,
				Snippet: truncateText(snippet, 300),
				Source:  "agent-browser",
			})
			if len(results) >= maxResults {
				return results
			}
		}
	}

	return results
}

func stripBrowserRefs(value string) string {
	return browserRefPattern.ReplaceAllString(value, "")
}

func normalizeWhitespace(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	return strings.TrimSpace(strings.Join(fields, " "))
}

func truncateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func mergeSearchResults(primary []searchResult, secondary []searchResult, limit int) []searchResult {
	merged := make([]searchResult, 0, minInt(limit, len(primary)+len(secondary)))
	seen := make(map[string]struct{})
	for _, group := range [][]searchResult{primary, secondary} {
		for _, item := range group {
			if _, exists := seen[item.URL]; exists {
				continue
			}
			seen[item.URL] = struct{}{}
			merged = append(merged, item)
			if len(merged) >= limit {
				return merged
			}
		}
	}
	return merged
}

func containsBlockedPhrase(text string) bool {
	text = strings.ToLower(text)
	return slices.ContainsFunc([]string{
		"enable javascript",
		"verify you are human",
		"access denied",
		"captcha",
	}, func(needle string) bool {
		return strings.Contains(text, needle)
	})
}
