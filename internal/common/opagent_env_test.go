package common

import (
	"strings"
	"testing"
)

func TestWithPrependedPathPrependsAndDeduplicates(t *testing.T) {
	env := []string{
		"PATH=/usr/bin:/bin:/tmp/opagent/bin",
		"HOME=/tmp/demo",
	}
	got := WithPrependedPath(env, "/tmp/opagent/bin", "/custom/bin")
	var pathValue string
	for _, item := range got {
		if strings.HasPrefix(item, "PATH=") {
			pathValue = strings.TrimPrefix(item, "PATH=")
			break
		}
	}
	if pathValue == "" {
		t.Fatal("PATH not found in env")
	}
	if strings.Count(pathValue, "/tmp/opagent/bin") != 1 {
		t.Fatalf("PATH = %q, want /tmp/opagent/bin only once", pathValue)
	}
	if !strings.HasPrefix(pathValue, "/tmp/opagent/bin:/custom/bin:") {
		t.Fatalf("PATH = %q, want prefixed opagent/custom bin entries", pathValue)
	}
}
