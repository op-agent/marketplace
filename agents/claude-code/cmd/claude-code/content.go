package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/op-agent/opagent-protocol/go-sdk/op"
)

func lookupEnv(key string) string {
	return os.Getenv(key)
}

func promptFromContent(content op.Content) (string, error) {
	switch value := content.(type) {
	case nil:
		return "", fmt.Errorf("request content is required")
	case *op.TextContent:
		return value.Text, nil
	case *op.JsonContent:
		return promptFromJSON(value.Raw)
	default:
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return "", fmt.Errorf("unsupported content type %T", content)
		}
		return string(encoded), nil
	}
}

func promptFromJSON(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", fmt.Errorf("empty JSON content")
	}

	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return "", fmt.Errorf("parse JSON content: %w", err)
	}

	if text := promptTextFromDecodedJSON(decoded); strings.TrimSpace(text) != "" {
		return text, nil
	}

	pretty, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		return string(trimmed), nil
	}
	return string(pretty), nil
}

func promptTextFromDecodedJSON(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := promptTextFromDecodedJSON(item); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		for _, key := range []string{"prompt", "message", "text", "input", "query", "instruction"} {
			if text, ok := typed[key].(string); ok && strings.TrimSpace(text) != "" {
				return text
			}
		}
		if content, ok := typed["content"]; ok {
			return promptTextFromDecodedJSON(content)
		}
		if messages, ok := typed["messages"]; ok {
			return promptTextFromDecodedJSON(messages)
		}
	}
	return ""
}

func cwdFromMeta(meta op.Meta) string {
	for _, key := range []string{"cwd", "CWD", "workspace", "workspaceRoot"} {
		if value, ok := meta[key]; ok {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}
