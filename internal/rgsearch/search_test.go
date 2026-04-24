package rgsearch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSearchParsesMatchOutputAndResolvesRelativePaths(t *testing.T) {
	root := t.TempDir()
	binary := writeSearchScript(t, "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"match\",\"data\":{\"path\":{\"text\":\"src/app.ts\"},\"lines\":{\"text\":\"const value = 42\\n\"},\"line_number\":3,\"submatches\":[{\"start\":6,\"end\":11}]}}'\n")

	result, err := Search(context.Background(), binary, Query{
		Root:    root,
		Pattern: "value",
	})
	if err != nil {
		t.Fatalf("Search(): %v", err)
	}
	if result.TotalCount != 1 {
		t.Fatalf("TotalCount = %d, want 1", result.TotalCount)
	}
	if got := result.Files[0].Path; got != filepath.Join(root, "src/app.ts") {
		t.Fatalf("path = %q, want %q", got, filepath.Join(root, "src/app.ts"))
	}
	if got := result.Files[0].Matches[0].Column; got != 7 {
		t.Fatalf("column = %d, want 7", got)
	}
}

func TestSearchMarksResultTruncatedWhenLimitReached(t *testing.T) {
	root := t.TempDir()
	binary := writeSearchScript(t, "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"match\",\"data\":{\"path\":{\"text\":\"a.txt\"},\"lines\":{\"text\":\"alpha\\n\"},\"line_number\":1,\"submatches\":[{\"start\":0,\"end\":5}]}}' '{\"type\":\"match\",\"data\":{\"path\":{\"text\":\"b.txt\"},\"lines\":{\"text\":\"beta\\n\"},\"line_number\":2,\"submatches\":[{\"start\":0,\"end\":4}]}}'\n")

	result, err := Search(context.Background(), binary, Query{
		Root:       root,
		Pattern:    "a",
		MaxMatches: 1,
	})
	if err != nil {
		t.Fatalf("Search(): %v", err)
	}
	if !result.Truncated {
		t.Fatal("Truncated = false, want true")
	}
	if result.TotalCount != 1 {
		t.Fatalf("TotalCount = %d, want 1", result.TotalCount)
	}
}

func writeSearchScript(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rg")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	return path
}
