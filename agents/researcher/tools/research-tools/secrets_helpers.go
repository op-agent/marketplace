package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var secretRefPattern = regexp.MustCompile(`@([a-zA-Z0-9_-]+)`)

// LoadSecretsFile reads a simple env-style secrets file.
// Lines are KEY=VALUE. Empty lines and lines starting with '#' are ignored.
func LoadSecretsFile(path string) (map[string]string, error) {
	secrets := make(map[string]string)
	if strings.TrimSpace(path) == "" {
		return secrets, nil
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return secrets, nil
		}
		return nil, fmt.Errorf("open secrets env: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid secrets env line %d", lineNum)
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, fmt.Errorf("empty key in secrets env line %d", lineNum)
		}
		value := strings.TrimSpace(parts[1])
		secrets[key] = stripSecretQuotes(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read secrets env: %w", err)
	}

	return secrets, nil
}

// ResolveSecretRefs replaces @id tokens using the provided secrets map.
func ResolveSecretRefs(value string, secrets map[string]string) (string, error) {
	if value == "" || !strings.Contains(value, "@") {
		return value, nil
	}

	missing := make(map[string]struct{})
	resolved := secretRefPattern.ReplaceAllStringFunc(value, func(match string) string {
		id := strings.TrimPrefix(match, "@")
		if v, ok := secrets[id]; ok {
			return v
		}
		missing[id] = struct{}{}
		return match
	})

	if len(missing) > 0 {
		ids := make([]string, 0, len(missing))
		for id := range missing {
			ids = append(ids, id)
		}
		return value, fmt.Errorf("missing secrets: %s", strings.Join(ids, ", "))
	}

	return resolved, nil
}

func stripSecretQuotes(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') ||
		(value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}
