package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/op-agent/opagent-protocol/go-sdk/op"
)

type envLookup func(string) string

type claudeConfig struct {
	BridgeMode          string
	CLI                 string
	OutputFormat        string
	Model               string
	PermissionMode      string
	AllowedTools        string
	DisallowedTools     string
	MaxTurns            string
	AppendOpAgentPrompt bool
	NotifyRawEvents     bool
	Timeout             time.Duration
}

type claudeRunInput struct {
	Prompt      string
	AgentPrompt string
	CWD         string
	BaseMeta    op.Meta
	Notify      func(context.Context, string, op.Meta) error
}

type claudeRunResult struct {
	AssistantText string
	PlainText     string
	FinalText     string
	ErrorText     string
	SessionID     string
	Model         string
	CWD           string
	IsError       bool
}

func claudeConfigFromEnv(getenv envLookup) claudeConfig {
	cfg := claudeConfig{
		BridgeMode:          firstEnv(getenv, "CLAUDE_CODE_BRIDGE_MODE"),
		CLI:                 firstEnv(getenv, "CLAUDE_CODE_CLI", "CLAUDE_CODE_COMMAND"),
		OutputFormat:        firstEnv(getenv, "CLAUDE_CODE_OUTPUT_FORMAT"),
		Model:               firstEnv(getenv, "CLAUDE_CODE_MODEL"),
		PermissionMode:      firstEnv(getenv, "CLAUDE_CODE_PERMISSION_MODE"),
		AllowedTools:        firstEnv(getenv, "CLAUDE_CODE_ALLOWED_TOOLS"),
		DisallowedTools:     firstEnv(getenv, "CLAUDE_CODE_DISALLOWED_TOOLS"),
		MaxTurns:            firstEnv(getenv, "CLAUDE_CODE_MAX_TURNS"),
		AppendOpAgentPrompt: envBool(getenv, "CLAUDE_CODE_APPEND_OPAGENT_PROMPT", envBool(getenv, "CLAUDE_CODE_APPEND_AGENT_PROMPT", true)),
		NotifyRawEvents:     envBool(getenv, "CLAUDE_CODE_NOTIFY_RAW_EVENTS", false),
	}
	if cfg.BridgeMode == "" {
		cfg.BridgeMode = "cli"
	}
	if cfg.CLI == "" {
		cfg.CLI = "claude"
	}
	if cfg.OutputFormat == "" {
		cfg.OutputFormat = "stream-json"
	}
	if cfg.PermissionMode == "" {
		cfg.PermissionMode = "yolo"
	}
	if seconds := strings.TrimSpace(firstEnv(getenv, "CLAUDE_CODE_TIMEOUT_SECONDS")); seconds != "" {
		if parsed, err := strconv.Atoi(seconds); err == nil && parsed > 0 {
			cfg.Timeout = time.Duration(parsed) * time.Second
		}
	}
	return cfg
}

func firstEnv(getenv envLookup, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func envBool(getenv envLookup, key string, fallback bool) bool {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func buildClaudeArgs(cfg claudeConfig, agentPrompt string) []string {
	args := []string{"--print"}

	if cfg.OutputFormat != "" && !strings.EqualFold(cfg.OutputFormat, "text") {
		args = append(args, "--output-format", cfg.OutputFormat)
		if strings.EqualFold(cfg.OutputFormat, "stream-json") {
			args = append(args, "--verbose")
		}
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	appendPermissionArgs(&args, cfg.PermissionMode)
	if cfg.AllowedTools != "" {
		args = append(args, "--allowedTools", cfg.AllowedTools)
	}
	if cfg.DisallowedTools != "" {
		args = append(args, "--disallowedTools", cfg.DisallowedTools)
	}
	if cfg.MaxTurns != "" {
		args = append(args, "--max-turns", cfg.MaxTurns)
	}
	if strings.TrimSpace(agentPrompt) != "" {
		args = append(args, "--append-system-prompt", agentPrompt)
	}

	return args
}

func appendPermissionArgs(args *[]string, mode string) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	switch normalized {
	case "", "none", "off", "false":
		return
	case "yolo", "bypass", "bypasspermissions", "bypass-permissions", "dangerously-skip-permissions", "skip":
		*args = append(*args, "--dangerously-skip-permissions")
	case "default":
		*args = append(*args, "--permission-mode", "default")
	default:
		*args = append(*args, "--permission-mode", mode)
	}
}

func runClaudeCLI(ctx context.Context, cfg claudeConfig, input claudeRunInput) (*claudeRunResult, error) {
	args := buildClaudeArgs(cfg, input.AgentPrompt)
	cmd := exec.CommandContext(ctx, cfg.CLI, args...)
	if strings.TrimSpace(input.CWD) != "" {
		cmd.Dir = input.CWD
	}
	cmd.Stdin = strings.NewReader(input.Prompt)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("prepare Claude Code stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("prepare Claude Code stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start Claude Code CLI %q: %w", cfg.CLI, err)
	}

	stderrCh := make(chan string, 1)
	go func() {
		stderrCh <- readLimited(stderr, 64*1024)
	}()

	result := &claudeRunResult{CWD: cmd.Dir}
	consumeErr := consumeClaudeOutput(ctx, stdout, cfg, input, result)
	waitErr := cmd.Wait()
	stderrText := <-stderrCh

	if consumeErr != nil {
		return result, consumeErr
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if waitErr != nil {
		return result, formatChildProcessError(waitErr, firstNonEmpty(result.ErrorText, result.FinalText, stderrText))
	}
	return result, nil
}

func consumeClaudeOutput(ctx context.Context, stdout io.Reader, cfg claudeConfig, input claudeRunInput, result *claudeRunResult) error {
	reader := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				if err := handleClaudeOutputLine(ctx, trimmed, cfg, input, result); err != nil {
					return err
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return fmt.Errorf("read Claude Code output: %w", readErr)
		}
	}
}

func handleClaudeOutputLine(ctx context.Context, line string, cfg claudeConfig, input claudeRunInput, result *claudeRunResult) error {
	summary, ok := summarizeClaudeEvent(line)
	if !ok {
		result.PlainText += line + "\n"
		return input.notify(ctx, line+"\n", input.BaseMeta.Add(op.Meta{"type": "stream", "claudeCode": map[string]any{"kind": "text"}}))
	}

	if summary.SessionID != "" {
		result.SessionID = summary.SessionID
	}
	if summary.Model != "" {
		result.Model = summary.Model
	}
	if summary.CWD != "" {
		result.CWD = summary.CWD
	}
	if summary.IsError {
		result.IsError = true
	}
	if summary.ErrorText != "" {
		result.ErrorText = summary.ErrorText
	}
	if summary.FinalText != "" {
		result.FinalText = summary.FinalText
	}
	if summary.AssistantText != "" {
		result.AssistantText += summary.AssistantText
		if err := input.notify(ctx, summary.AssistantText, input.BaseMeta.Add(op.Meta{"type": "stream", "claudeCode": map[string]any{"kind": "assistant"}})); err != nil {
			return err
		}
	}
	if summary.ProgressText != "" {
		if err := input.notify(ctx, summary.ProgressText, input.BaseMeta.Add(op.Meta{"type": "stream", "claudeCode": map[string]any{"kind": summary.Kind}})); err != nil {
			return err
		}
	}
	if cfg.NotifyRawEvents {
		return input.notify(ctx, line+"\n", input.BaseMeta.Add(op.Meta{"type": "ignore", "claudeCode": map[string]any{"kind": "raw"}}))
	}
	return nil
}

func (input claudeRunInput) notify(ctx context.Context, message string, meta op.Meta) error {
	if input.Notify == nil {
		return nil
	}
	return input.Notify(ctx, message, meta)
}

func readLimited(reader io.Reader, limit int64) string {
	writer := &truncatingBuffer{limit: limit}
	_, _ = io.Copy(writer, reader)
	text := writer.String()
	if writer.truncated {
		text += "\n... stderr truncated ..."
	}
	return text
}

type truncatingBuffer struct {
	buffer    bytes.Buffer
	limit     int64
	seen      int64
	truncated bool
}

func (w *truncatingBuffer) Write(p []byte) (int, error) {
	w.seen += int64(len(p))
	if w.limit <= 0 {
		w.truncated = true
		return len(p), nil
	}
	remaining := w.limit - int64(w.buffer.Len())
	if remaining <= 0 {
		w.truncated = true
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		w.buffer.Write(p[:remaining])
		w.truncated = true
		return len(p), nil
	}
	w.buffer.Write(p)
	return len(p), nil
}

func (w *truncatingBuffer) String() string {
	return w.buffer.String()
}

func formatChildProcessError(err error, stderr string) error {
	message := strings.TrimSpace(redactSensitive(stderr))
	if message == "" {
		message = err.Error()
	}
	return fmt.Errorf("Claude Code CLI failed: %w: %s", err, message)
}

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|token|password|secret)\s*[:=]\s*[^\s]+`),
	regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]+`),
}

func redactSensitive(text string) string {
	redacted := text
	for _, pattern := range sensitivePatterns {
		redacted = pattern.ReplaceAllStringFunc(redacted, func(match string) string {
			if strings.HasPrefix(strings.ToLower(match), "sk-ant-") {
				return "sk-ant-<redacted>"
			}
			parts := strings.FieldsFunc(match, func(r rune) bool { return r == ':' || r == '=' })
			if len(parts) == 0 {
				return "<redacted>"
			}
			separator := "="
			if strings.Contains(match, ":") {
				separator = ":"
			}
			return strings.TrimSpace(parts[0]) + separator + "<redacted>"
		})
	}
	const maxLen = 4000
	if len(redacted) > maxLen {
		return redacted[:maxLen] + "\n... truncated ..."
	}
	return redacted
}
