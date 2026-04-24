package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const BinaryNotFoundMessage = "ripgrep binary not found"

type MissingBinaryError struct {
	Candidates []string
}

func (e *MissingBinaryError) Error() string {
	if len(e.Candidates) == 0 {
		return BinaryNotFoundMessage
	}
	return fmt.Sprintf("%s: %s", BinaryNotFoundMessage, strings.Join(e.Candidates, ", "))
}

func IsMissingBinaryError(err error) bool {
	var target *MissingBinaryError
	return errors.As(err, &target)
}

func ResolveBinary(candidates ...string) (string, error) {
	tried := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		tried = append(tried, candidate)
		if looksLikePath(candidate) {
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() {
				return candidate, nil
			}
			continue
		}
		path, err := exec.LookPath(candidate)
		if err == nil && strings.TrimSpace(path) != "" {
			return path, nil
		}
	}
	return "", &MissingBinaryError{Candidates: tried}
}

func looksLikePath(value string) bool {
	if value == "" {
		return false
	}
	return filepath.IsAbs(value) || strings.ContainsRune(value, os.PathSeparator)
}
