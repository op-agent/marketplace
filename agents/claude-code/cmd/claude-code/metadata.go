package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/op-agent/opagent-protocol/go-sdk/op"
	"gopkg.in/yaml.v3"
)

type agentFrontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func resolveAgentFile() (string, error) {
	if value := strings.TrimSpace(os.Getenv("OPAGENT_AGENT_FILE")); value != "" {
		if fileExists(value) {
			return value, nil
		}
		return "", fmt.Errorf("OPAGENT_AGENT_FILE does not exist: %s", value)
	}

	candidates := make([]string, 0, 8)
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "..", ".agent", "AGENT.md"),
			filepath.Join(exeDir, ".agent", "AGENT.md"),
			filepath.Join(exeDir, "..", "AGENT.md"),
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, ".agent", "AGENT.md"),
			filepath.Join(cwd, "AGENT.md"),
		)
		for dir := cwd; ; dir = filepath.Dir(dir) {
			candidates = append(candidates, filepath.Join(dir, "agents", agentID, "AGENT.md"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}

	for _, candidate := range candidates {
		cleaned := filepath.Clean(candidate)
		if fileExists(cleaned) {
			return cleaned, nil
		}
	}
	return "", fmt.Errorf("AGENT.md not found for %s", agentID)
}

func loadAgentMeta(agentFile string) (*op.AgentMeta, error) {
	raw, err := os.ReadFile(agentFile)
	if err != nil {
		return nil, fmt.Errorf("read agent metadata: %w", err)
	}
	frontMatter, err := parseAgentFrontMatter(string(raw))
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(frontMatter.Name)
	if name == "" {
		name = "Claude Code"
	}
	description := strings.TrimSpace(frontMatter.Description)
	if description == "" {
		description = "Claude Code bridge for workspace-aware coding tasks."
	}
	return &op.AgentMeta{
		Name:        name,
		Description: description,
	}, nil
}

func loadAgentPrompt(agentFile string) (string, error) {
	raw, err := os.ReadFile(agentFile)
	if err != nil {
		return "", fmt.Errorf("read agent prompt: %w", err)
	}
	body, err := markdownBody(string(raw))
	if err != nil {
		return "", err
	}
	return expandPromptVariables(body), nil
}

func parseAgentFrontMatter(markdown string) (agentFrontMatter, error) {
	front, _, ok := splitFrontMatter(markdown)
	if !ok {
		return agentFrontMatter{}, nil
	}
	var parsed agentFrontMatter
	if err := yaml.Unmarshal([]byte(front), &parsed); err != nil {
		return agentFrontMatter{}, fmt.Errorf("parse agent front matter: %w", err)
	}
	return parsed, nil
}

func markdownBody(markdown string) (string, error) {
	_, body, ok := splitFrontMatter(markdown)
	if !ok {
		return strings.TrimSpace(markdown), nil
	}
	return strings.TrimSpace(body), nil
}

func splitFrontMatter(markdown string) (frontMatter string, body string, ok bool) {
	text := strings.TrimPrefix(strings.ReplaceAll(markdown, "\r\n", "\n"), "\ufeff")
	if !strings.HasPrefix(text, "---\n") {
		return "", markdown, false
	}
	remaining := text[len("---\n"):]
	idx := strings.Index(remaining, "\n---")
	if idx < 0 {
		return "", markdown, false
	}
	front := remaining[:idx]
	bodyStart := idx + len("\n---")
	if len(remaining) > bodyStart && remaining[bodyStart] == '\n' {
		bodyStart++
	}
	return strings.TrimSpace(front), remaining[bodyStart:], true
}

func expandPromptVariables(prompt string) string {
	replacements := map[string]string{
		"${platform}":  platformName(),
		"{{platform}}": platformName(),
	}
	out := prompt
	for placeholder, value := range replacements {
		out = strings.ReplaceAll(out, placeholder, value)
	}
	return out
}

func platformName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
