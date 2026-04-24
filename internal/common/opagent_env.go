package common

import (
	"os"
	"path/filepath"
	"strings"
)

func OpagentBinDir() string {
	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".opagent", "bin")
	}
	if home = strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return filepath.Join(home, ".opagent", "bin")
	}
	return filepath.Join(".opagent", "bin")
}

func WithPrependedPath(env []string, dirs ...string) []string {
	baseEnv := env
	if len(baseEnv) == 0 {
		baseEnv = os.Environ()
	}

	pathEntries := make([]string, 0, len(dirs)+4)
	seen := make(map[string]struct{})
	for _, dir := range dirs {
		trimmed := strings.TrimSpace(dir)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		pathEntries = append(pathEntries, trimmed)
	}

	out := make([]string, 0, len(baseEnv)+1)
	hasPath := false
	for _, item := range baseEnv {
		if strings.HasPrefix(item, "PATH=") {
			hasPath = true
			current := strings.TrimPrefix(item, "PATH=")
			for _, part := range filepath.SplitList(current) {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				if _, ok := seen[part]; ok {
					continue
				}
				seen[part] = struct{}{}
				pathEntries = append(pathEntries, part)
			}
			continue
		}
		out = append(out, item)
	}

	if !hasPath {
		for _, part := range filepath.SplitList(strings.TrimSpace(os.Getenv("PATH"))) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			pathEntries = append(pathEntries, part)
		}
	}

	out = append(out, "PATH="+strings.Join(pathEntries, string(os.PathListSeparator)))
	return out
}
