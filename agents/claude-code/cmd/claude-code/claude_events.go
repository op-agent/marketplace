package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type claudeEventSummary struct {
	Kind          string
	AssistantText string
	ProgressText  string
	FinalText     string
	ErrorText     string
	SessionID     string
	Model         string
	CWD           string
	IsError       bool
}

func summarizeClaudeEvent(line string) (claudeEventSummary, bool) {
	var event map[string]any
	decoder := json.NewDecoder(strings.NewReader(line))
	decoder.UseNumber()
	if err := decoder.Decode(&event); err != nil {
		return claudeEventSummary{}, false
	}

	summary := claudeEventSummary{Kind: stringField(event, "type")}
	summary.SessionID = firstStringField(event, "session_id", "sessionID")
	summary.Model = stringField(event, "model")
	summary.CWD = firstStringField(event, "cwd", "working_directory")

	switch summary.Kind {
	case "assistant":
		message, _ := event["message"].(map[string]any)
		if message == nil {
			message = event
		}
		textBlocks := extractTextBlocks(message["content"])
		if len(textBlocks) == 0 {
			textBlocks = extractTextBlocks(event["content"])
		}
		summary.AssistantText = strings.Join(textBlocks, "")
		if progress := summarizeToolUses(message["content"]); progress != "" {
			summary.ProgressText = progress
		}
	case "system":
		summary.Kind = firstNonEmpty(stringField(event, "subtype"), "system")
		if summary.Model == "" {
			summary.Model = stringField(event, "model")
		}
		if summary.CWD == "" {
			summary.CWD = stringField(event, "cwd")
		}
		if summary.Kind == "init" {
			parts := []string{"\n\nℹ️ Claude Code session initialized"}
			if summary.Model != "" {
				parts = append(parts, "model: `"+summary.Model+"`")
			}
			if summary.CWD != "" {
				parts = append(parts, "cwd: `"+summary.CWD+"`")
			}
			if summary.SessionID != "" {
				parts = append(parts, "session: `"+summary.SessionID+"`")
			}
			if len(parts) == 1 {
				summary.ProgressText = parts[0] + ".\n\n"
			} else {
				summary.ProgressText = parts[0] + " (" + strings.Join(parts[1:], ", ") + ").\n\n"
			}
		}
	case "result":
		summary.FinalText = stringField(event, "result")
		if isJSONTruthy(event["is_error"]) || strings.EqualFold(stringField(event, "subtype"), "error") {
			summary.IsError = true
			summary.ErrorText = firstNonEmpty(stringField(event, "error"), stringField(event, "message"), summary.FinalText)
		}
	case "error":
		summary.IsError = true
		summary.ErrorText = firstNonEmpty(stringField(event, "error"), stringField(event, "message"), compactJSON(event))
	case "user":
		if progress := summarizeToolResults(event); progress != "" {
			summary.ProgressText = progress
		}
	default:
		if progress := summarizeToolUses(event["content"]); progress != "" {
			summary.ProgressText = progress
		}
	}

	if summary.Kind == "" {
		summary.Kind = "event"
	}
	return summary, true
}

func extractTextBlocks(value any) []string {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	case []any:
		var out []string
		for _, item := range typed {
			out = append(out, extractTextBlocks(item)...)
		}
		return out
	case map[string]any:
		blockType := stringField(typed, "type")
		switch blockType {
		case "text", "thinking":
			if text := stringField(typed, "text"); text != "" {
				return []string{text}
			}
		case "tool_use", "tool_result":
			return nil
		}
		if text := stringField(typed, "text"); text != "" && blockType == "" {
			return []string{text}
		}
	}
	return nil
}

func summarizeToolUses(value any) string {
	blocks, ok := value.([]any)
	if !ok {
		return ""
	}
	var lines []string
	for _, block := range blocks {
		obj, ok := block.(map[string]any)
		if !ok || stringField(obj, "type") != "tool_use" {
			continue
		}
		name := firstNonEmpty(stringField(obj, "name"), "tool")
		input := compactJSON(obj["input"])
		if input != "" {
			input = " " + truncateForProgress(input, 300)
		}
		lines = append(lines, fmt.Sprintf("\n\n⚙️ Claude Code is using `%s`%s\n\n", name, input))
	}
	return strings.Join(lines, "")
}

func summarizeToolResults(event map[string]any) string {
	message, _ := event["message"].(map[string]any)
	if message == nil {
		message = event
	}
	blocks, ok := message["content"].([]any)
	if !ok {
		return ""
	}
	count := 0
	for _, block := range blocks {
		obj, ok := block.(map[string]any)
		if ok && stringField(obj, "type") == "tool_result" {
			count++
		}
	}
	if count == 0 {
		return ""
	}
	if count == 1 {
		return "\n\n✅ Claude Code received a tool result.\n\n"
	}
	return fmt.Sprintf("\n\n✅ Claude Code received %d tool results.\n\n", count)
}

func stringField(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	value, ok := obj[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case fmt.Stringer:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func firstStringField(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringField(obj, key); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func isJSONTruthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes":
			return true
		}
	case json.Number:
		return typed.String() != "0"
	case float64:
		return typed != 0
	}
	return false
}

func compactJSON(value any) string {
	if value == nil {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return strings.TrimSpace(fmt.Sprint(value))
	}
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, raw); err != nil {
		return string(raw)
	}
	return buffer.String()
}

func truncateForProgress(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "…"
}
