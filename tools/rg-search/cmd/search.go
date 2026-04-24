package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultMaxFiles   = 200
	defaultMaxMatches = 500
	scannerBufferSize = 1024 * 1024
)

type eventEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type matchEvent struct {
	Path       textField  `json:"path"`
	Lines      textField  `json:"lines"`
	LineNumber int        `json:"line_number"`
	Submatches []submatch `json:"submatches"`
}

type textField struct {
	Text string `json:"text"`
}

type submatch struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

func Search(ctx context.Context, binary string, query Query) (Result, error) {
	root := strings.TrimSpace(query.Root)
	pattern := strings.TrimSpace(query.Pattern)
	if root == "" {
		return Result{}, fmt.Errorf("search root is required")
	}
	if pattern == "" {
		return Result{}, fmt.Errorf("search pattern is required")
	}
	info, err := os.Stat(root)
	if err != nil {
		return Result{}, fmt.Errorf("search root: %w", err)
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("search root is not a directory: %s", root)
	}
	if strings.TrimSpace(binary) == "" {
		return Result{}, &MissingBinaryError{}
	}

	maxFiles := query.MaxFiles
	if maxFiles <= 0 {
		maxFiles = defaultMaxFiles
	}
	maxMatches := query.MaxMatches
	if maxMatches <= 0 {
		maxMatches = defaultMaxMatches
	}

	searchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	args := buildArgs(query, root)
	cmd := exec.CommandContext(searchCtx, binary, args...)
	cmd.Dir = root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("rg stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, fmt.Errorf("rg stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start rg: %w", err)
	}

	result := Result{
		Files: make([]FileResult, 0),
	}
	fileIndex := make(map[string]int)
	truncated := false

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), scannerBufferSize)
	for scanner.Scan() {
		var envelope eventEnvelope
		if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
			continue
		}
		if envelope.Type != "match" {
			continue
		}
		var match matchEvent
		if err := json.Unmarshal(envelope.Data, &match); err != nil {
			continue
		}
		path := strings.TrimSpace(match.Path.Text)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		idx, ok := fileIndex[path]
		if !ok {
			if len(result.Files) >= maxFiles {
				truncated = true
				cancel()
				break
			}
			fileIndex[path] = len(result.Files)
			result.Files = append(result.Files, FileResult{
				Path:    path,
				Matches: make([]Match, 0, 4),
			})
			idx = len(result.Files) - 1
		}
		file := &result.Files[idx]
		next := toMatch(match)
		file.Matches = append(file.Matches, next)
		file.Count++
		result.TotalCount++
		if result.TotalCount >= maxMatches {
			truncated = true
			cancel()
			break
		}
	}
	if err := scanner.Err(); err != nil && !truncated {
		return Result{}, fmt.Errorf("read rg output: %w", err)
	}

	stderrBytes, stderrErr := ioReadAll(stderr)
	waitErr := cmd.Wait()
	if stderrErr != nil && !truncated {
		return Result{}, fmt.Errorf("read rg stderr: %w", stderrErr)
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			if exitErr.ExitCode() == 1 {
				waitErr = nil
			} else if truncated && errors.Is(searchCtx.Err(), context.Canceled) {
				waitErr = nil
			}
		} else if truncated && errors.Is(searchCtx.Err(), context.Canceled) {
			waitErr = nil
		}
	}
	if waitErr != nil {
		stderrText := strings.TrimSpace(string(stderrBytes))
		if stderrText != "" {
			return Result{}, fmt.Errorf("rg failed: %w: %s", waitErr, stderrText)
		}
		return Result{}, fmt.Errorf("rg failed: %w", waitErr)
	}

	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].Path < result.Files[j].Path
	})
	result.Truncated = truncated
	return result, nil
}

func buildArgs(query Query, root string) []string {
	args := []string{"--json", "--line-number", "--column", "--color", "never"}
	if !query.Regex {
		args = append(args, "-F")
	}
	if !query.MatchCase {
		args = append(args, "-i")
	}
	if query.WholeWord {
		args = append(args, "-w")
	}
	for _, include := range query.Includes {
		include = strings.TrimSpace(include)
		if include != "" {
			args = append(args, "-g", include)
		}
	}
	for _, exclude := range query.Excludes {
		exclude = strings.TrimSpace(exclude)
		if exclude != "" {
			args = append(args, "-g", "!"+exclude)
		}
	}
	args = append(args, query.Pattern, root)
	return args
}

func toMatch(event matchEvent) Match {
	text := strings.TrimRight(event.Lines.Text, "\r\n")
	column := 1
	endColumn := 1
	if len(event.Submatches) > 0 {
		column = event.Submatches[0].Start + 1
		endColumn = max(column, event.Submatches[0].End+1)
	}
	return Match{
		Line:      max(1, event.LineNumber),
		Column:    column,
		EndColumn: endColumn,
		Text:      text,
	}
}

func ioReadAll(reader io.ReadCloser) ([]byte, error) {
	if reader == nil {
		return nil, nil
	}
	defer reader.Close()
	return io.ReadAll(reader)
}
