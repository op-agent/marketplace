package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type toolConfig struct {
	TavilyBaseURL            string
	TavilyAPIKey             string
	JinaReaderPrefix         string
	JinaAPIKey               string
	BrowserCommand           string
	BrowserSearchURLTemplate string
	BrowserOpenArgs          []string
	BrowserSnapshotArgs      []string
	UserAgent                string
	MaxFetchChars            int
}

type rawToolConfig struct {
	TavilyBaseURL            string   `json:"tavily_base_url"`
	TavilyAPIKey             string   `json:"tavily_api_key"`
	JinaReaderPrefix         string   `json:"jina_reader_prefix"`
	JinaAPIKey               string   `json:"jina_api_key"`
	BrowserCommand           string   `json:"browser_command"`
	BrowserSearchURLTemplate string   `json:"browser_search_url_template"`
	BrowserOpenArgs          []string `json:"browser_open_args"`
	BrowserSnapshotArgs      []string `json:"browser_snapshot_args"`
	UserAgent                string   `json:"user_agent"`
	MaxFetchChars            int      `json:"max_fetch_chars"`
}

func defaultToolConfig() toolConfig {
	return toolConfig{
		TavilyBaseURL:            "https://api.tavily.com/search",
		JinaReaderPrefix:         "https://r.jina.ai/http://",
		BrowserCommand:           "agent-browser",
		BrowserSearchURLTemplate: "https://duckduckgo.com/?q=%s",
		BrowserOpenArgs:          []string{"open"},
		BrowserSnapshotArgs:      []string{"snapshot", "-i"},
		UserAgent:                "ResearcherBot/1.0 (+https://www.opagent.io)",
		MaxFetchChars:            16000,
	}
}

func loadToolConfig(workdir string) (toolConfig, error) {
	cfg := defaultToolConfig()

	rawPath := filepath.Join(workdir, "config.json")
	content, err := os.ReadFile(rawPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config.json: %w", err)
	}

	var raw rawToolConfig
	if err := json.Unmarshal(content, &raw); err != nil {
		return cfg, fmt.Errorf("parse config.json: %w", err)
	}

	secretsPath, err := defaultSecretsPath()
	if err != nil {
		return cfg, err
	}
	secrets, err := LoadSecretsFile(secretsPath)
	if err != nil {
		return cfg, err
	}

	if cfg.TavilyBaseURL, err = resolveConfiguredString(raw.TavilyBaseURL, cfg.TavilyBaseURL, secrets); err != nil {
		return cfg, err
	}
	if cfg.TavilyAPIKey, err = resolveOptionalConfiguredString(raw.TavilyAPIKey, cfg.TavilyAPIKey, secrets); err != nil {
		return cfg, err
	}
	if cfg.JinaReaderPrefix, err = resolveConfiguredString(raw.JinaReaderPrefix, cfg.JinaReaderPrefix, secrets); err != nil {
		return cfg, err
	}
	if cfg.JinaAPIKey, err = resolveOptionalConfiguredString(raw.JinaAPIKey, cfg.JinaAPIKey, secrets); err != nil {
		return cfg, err
	}
	if cfg.BrowserCommand, err = resolveConfiguredString(raw.BrowserCommand, cfg.BrowserCommand, secrets); err != nil {
		return cfg, err
	}
	if cfg.BrowserSearchURLTemplate, err = resolveConfiguredString(raw.BrowserSearchURLTemplate, cfg.BrowserSearchURLTemplate, secrets); err != nil {
		return cfg, err
	}
	if cfg.UserAgent, err = resolveConfiguredString(raw.UserAgent, cfg.UserAgent, secrets); err != nil {
		return cfg, err
	}
	if len(raw.BrowserOpenArgs) > 0 {
		cfg.BrowserOpenArgs = append([]string(nil), raw.BrowserOpenArgs...)
	}
	if len(raw.BrowserSnapshotArgs) > 0 {
		cfg.BrowserSnapshotArgs = append([]string(nil), raw.BrowserSnapshotArgs...)
	}
	if raw.MaxFetchChars > 0 {
		cfg.MaxFetchChars = raw.MaxFetchChars
	}

	cfg.TavilyBaseURL = strings.TrimSpace(cfg.TavilyBaseURL)
	cfg.JinaReaderPrefix = strings.TrimSpace(cfg.JinaReaderPrefix)
	cfg.BrowserCommand = strings.TrimSpace(cfg.BrowserCommand)
	cfg.BrowserSearchURLTemplate = strings.TrimSpace(cfg.BrowserSearchURLTemplate)
	cfg.UserAgent = strings.TrimSpace(cfg.UserAgent)

	return cfg, nil
}

func resolveConfiguredString(raw string, fallback string, secrets map[string]string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	return ResolveSecretRefs(raw, secrets)
}

func resolveOptionalConfiguredString(raw string, fallback string, secrets map[string]string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := ResolveSecretRefs(raw, secrets)
	if err != nil && strings.Contains(err.Error(), "missing secrets:") {
		return fallback, nil
	}
	return value, err
}

func defaultSecretsPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(homeDir, ".opagent", "configs", "secrets.env"), nil
}
