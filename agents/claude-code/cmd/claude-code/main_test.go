package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/op-agent/opagent-protocol/go-sdk/op"
)

func TestClaudeConfigFromEnvDefaultsAndAliases(t *testing.T) {
	values := map[string]string{
		"CLAUDE_CODE_COMMAND":               "/custom/claude",
		"CLAUDE_CODE_MODEL":                 "opus",
		"CLAUDE_CODE_APPEND_OPAGENT_PROMPT": "false",
		"CLAUDE_CODE_TIMEOUT_SECONDS":       "12",
	}
	cfg := claudeConfigFromEnv(func(key string) string { return values[key] })
	if cfg.BridgeMode != "cli" {
		t.Fatalf("BridgeMode = %q, want cli", cfg.BridgeMode)
	}
	if cfg.CLI != "/custom/claude" {
		t.Fatalf("CLI = %q", cfg.CLI)
	}
	if cfg.OutputFormat != "stream-json" {
		t.Fatalf("OutputFormat = %q", cfg.OutputFormat)
	}
	if cfg.PermissionMode != "yolo" {
		t.Fatalf("PermissionMode = %q", cfg.PermissionMode)
	}
	if cfg.AppendOpAgentPrompt {
		t.Fatalf("AppendOpAgentPrompt = true, want false")
	}
	if cfg.Timeout.String() != "12s" {
		t.Fatalf("Timeout = %s", cfg.Timeout)
	}
}

func TestBuildClaudeArgs(t *testing.T) {
	args := buildClaudeArgs(claudeConfig{
		OutputFormat:    "stream-json",
		Model:           "sonnet",
		PermissionMode:  "yolo",
		AllowedTools:    "Read,Edit",
		DisallowedTools: "Bash",
		MaxTurns:        "8",
	}, "agent prompt")
	want := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--model", "sonnet",
		"--dangerously-skip-permissions",
		"--allowedTools", "Read,Edit",
		"--disallowedTools", "Bash",
		"--max-turns", "8",
		"--append-system-prompt", "agent prompt",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args mismatch\n got: %#v\nwant: %#v", args, want)
	}
}

func TestPromptFromContent(t *testing.T) {
	text, err := promptFromContent(&op.TextContent{Text: "hello"})
	if err != nil || text != "hello" {
		t.Fatalf("text prompt = %q, err=%v", text, err)
	}

	jsonText, err := promptFromContent(&op.JsonContent{Raw: json.RawMessage(`{"messages":[{"role":"user","content":"build this"}]}`)})
	if err != nil {
		t.Fatalf("json prompt error: %v", err)
	}
	if jsonText != "build this" {
		t.Fatalf("json prompt = %q", jsonText)
	}
}

func TestSummarizeClaudeEvent(t *testing.T) {
	line := `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[{"type":"text","text":"Hello"},{"type":"tool_use","name":"Read","input":{"file_path":"main.go"}}]}}}`
	summary, ok := summarizeClaudeEvent(line)
	if !ok {
		t.Fatalf("event was not parsed")
	}
	if summary.SessionID != "s1" || summary.AssistantText != "Hello" {
		t.Fatalf("summary = %#v", summary)
	}
	if !strings.Contains(summary.ProgressText, "Read") || !strings.Contains(summary.ProgressText, "main.go") {
		t.Fatalf("progress text = %q", summary.ProgressText)
	}
}

func TestRunClaudeCLIAggregatesStreamJSON(t *testing.T) {
	fake := writeFakeClaude(t, `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"type":"system","subtype":"init","session_id":"sess-1","model":"sonnet","cwd":"/workspace"}'
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"partial"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","result":"final answer"}'
`)
	var notifications []string
	result, err := runClaudeCLI(context.Background(), claudeConfig{CLI: fake, OutputFormat: "stream-json", PermissionMode: "none"}, claudeRunInput{
		Prompt:   "hello",
		BaseMeta: op.Meta{"threadID": "t1"},
		Notify: func(_ context.Context, message string, _ op.Meta) error {
			notifications = append(notifications, message)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runClaudeCLI error: %v", err)
	}
	if result.FinalText != "final answer" || result.AssistantText != "partial" || result.SessionID != "sess-1" || result.Model != "sonnet" {
		t.Fatalf("result = %#v", result)
	}
	if len(notifications) != 2 || !strings.Contains(notifications[0], "session initialized") || notifications[1] != "partial" {
		t.Fatalf("notifications = %#v", notifications)
	}
}

func TestRunClaudeCLIRedactsChildProcessErrors(t *testing.T) {
	fake := writeFakeClaude(t, `#!/bin/sh
cat >/dev/null
printf '%s\n' 'api_key=sk-ant-test-123' >&2
exit 7
`)
	_, err := runClaudeCLI(context.Background(), claudeConfig{CLI: fake, OutputFormat: "stream-json", PermissionMode: "none"}, claudeRunInput{Prompt: "hello"})
	if err == nil {
		t.Fatalf("expected error")
	}
	text := err.Error()
	if strings.Contains(text, "sk-ant-test-123") {
		t.Fatalf("error leaked sensitive value: %s", text)
	}
	if !strings.Contains(text, "<redacted>") {
		t.Fatalf("error did not include redaction marker: %s", text)
	}
}

func TestRunClaudeCLIUsesStructuredErrorBeforeStderr(t *testing.T) {
	fake := writeFakeClaude(t, `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"type":"result","is_error":true,"result":"auth failed"}'
printf '%s\n' 'exit status 1' >&2
exit 1
`)
	_, err := runClaudeCLI(context.Background(), claudeConfig{CLI: fake, OutputFormat: "stream-json", PermissionMode: "none"}, claudeRunInput{Prompt: "hello"})
	if err == nil {
		t.Fatalf("expected error")
	}
	text := err.Error()
	if !strings.Contains(text, "auth failed") {
		t.Fatalf("error did not include structured result: %s", text)
	}
}

func TestLoadAgentMetaAndPrompt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENT.md")
	if err := os.WriteFile(path, []byte("---\nname: Demo\ndescription: Demo agent\n---\nhello ${platform}\n"), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}
	meta, err := loadAgentMeta(path)
	if err != nil {
		t.Fatalf("loadAgentMeta error: %v", err)
	}
	if meta.Name != "Demo" || meta.Description != "Demo agent" {
		t.Fatalf("meta = %#v", meta)
	}
	prompt, err := loadAgentPrompt(path)
	if err != nil {
		t.Fatalf("loadAgentPrompt error: %v", err)
	}
	if !strings.Contains(prompt, platformName()) {
		t.Fatalf("prompt = %q", prompt)
	}
}

func TestSplitFrontMatterAndPromptExpansion(t *testing.T) {
	body, err := markdownBody("---\nname: Test\n---\nhello ${platform}\n")
	if err != nil {
		t.Fatalf("markdownBody error: %v", err)
	}
	got := expandPromptVariables(body)
	if !strings.Contains(got, platformName()) {
		t.Fatalf("expanded body = %q, GOOS=%s", got, runtime.GOOS)
	}
}

func writeFakeClaude(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	return path
}
