package common

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveAbsolutePath returns a reliable absolute path based on workDir and path.
// - Supports ~ and ~/ prefix expansion
// - If workDir is empty, uses current working directory
// - If path is absolute, returns the cleaned result
// - Otherwise, joins workDir with path and returns the cleaned result
func ResolveAbsolutePath(workDir, path string) (string, error) {
	if path == "" {
		return "", nil
	}

	expanded, err := ExpandHome(path)
	if err != nil {
		return "", err
	}
	path = expanded

	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}

	if workDir == "" {
		workDir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	return filepath.Clean(filepath.Join(workDir, path)), nil
}

func ExpandHome(path string) (string, error) {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}

	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}

	return path, nil
}

// LooksLikePath checks if a string looks like a file or directory path.
// Cross-platform support for both Unix and Windows paths.
// Returns true if the string:
// - Is an absolute path (Unix: /root, Windows: C:\)
// - Starts with "~", "~/" or "~\" (home directory path)
// - Starts with "./" or ".\" or "../" or "..\" (relative path)
// - Contains path separator "/" or "\"
// Returns false for command-line flags (strings starting with "-")
func LooksLikePath(s string) bool {
	if s == "" {
		return false
	}

	// Command-line flags are not paths
	if strings.HasPrefix(s, "-") {
		return false
	}

	// Home directory paths
	if s == "~" || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, "~\\") {
		return true
	}

	// Cross-platform absolute path detection (handles C:\, /root, etc.)
	if filepath.IsAbs(s) {
		return true
	}

	// Relative paths with explicit prefix (both / and \)
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, ".\\") ||
		strings.HasPrefix(s, "../") || strings.HasPrefix(s, "..\\") {
		return true
	}

	// Contains any path separator
	if strings.ContainsAny(s, `/\`) {
		return true
	}

	return false
}
