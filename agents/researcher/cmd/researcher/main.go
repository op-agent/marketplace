package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/op-agent/opagent-protocol/go-sdk/op"
	"gopkg.in/yaml.v3"
)

type agentConfig struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type listedNode struct {
	Key  string          `json:"key"`
	Kind string          `json:"kind"`
	URI  string          `json:"uri"`
	Cwd  string          `json:"cwd"`
	Meta json.RawMessage `json:"meta"`
}

type listedAgentMeta struct {
	Skills []string `json:"skills"`
}

type listedSkillMeta struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func main() {
	agentFile, meta, err := loadAgentRegistrationMeta()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load researcher agent metadata: %v\n", err)
		os.Exit(1)
	}

	server := op.NewServer(&op.Implementation{Name: meta.Name, Version: "v1.0.0"}, &op.ServerOptions{
		OpNodeHandler: func(ctx context.Context, req *op.ServerRequest[*op.OpNodeParams]) (*op.OpNodeResult, error) {
			return handleOpNode(ctx, req, agentFile)
		},
	})
	server.AddAgent(meta, handleCallAgent)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer stop()

	if err := server.Run(ctx, &op.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "researcher agent server failed: %v\n", err)
		os.Exit(1)
	}
}

func handleCallAgent(ctx context.Context, req *op.CallAgentRequest) (*op.CallAgentResult, error) {
	if req == nil || req.Session == nil || req.Params == nil {
		return nil, fmt.Errorf("agent call params are required")
	}
	result, err := req.Session.OpNode(ctx, &op.OpNodeParams{
		OpCode:  op.OpAgentLoopCreate,
		Meta:    cloneMeta(req.Params.Meta),
		Content: req.Params.Content,
	})
	if err != nil {
		return nil, err
	}
	return &op.CallAgentResult{
		AgentID: req.Params.AgentID,
		Meta:    result.Meta,
		Content: result.Content,
	}, nil
}

func handleOpNode(ctx context.Context, req *op.ServerRequest[*op.OpNodeParams], agentFile string) (*op.OpNodeResult, error) {
	if req == nil || req.Params == nil {
		return nil, fmt.Errorf("node operation params are required")
	}
	switch req.Params.OpCode {
	case op.OpPromptGet:
		prompt, err := buildPrompt(ctx, req.Session, agentFile, req.Params.Meta)
		if err != nil {
			return nil, err
		}
		return &op.OpNodeResult{
			OpCode:  op.OpPromptGet,
			Meta:    cloneMeta(req.Params.Meta),
			Content: &op.TextContent{Text: prompt},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported node opcode: %s", req.Params.OpCode)
	}
}

func buildPrompt(ctx context.Context, session *op.ServerSession, agentFile string, meta op.Meta) (string, error) {
	basePrompt, err := LoadMarkdownBody(agentFile)
	if err != nil {
		return "", fmt.Errorf("load agent prompt: %w", err)
	}
	basePrompt = ExpandPlatformVariables(basePrompt, runtime.GOOS)

	cwdAgents, err := ReadCwdAgentsContext(metaString(meta, "cwd"))
	if err != nil {
		return "", fmt.Errorf("read cwd AGENTS.md: %w", err)
	}

	availableSkills, selectedSkills, err := resolveSkillContexts(ctx, session, meta)
	if err != nil {
		return "", fmt.Errorf("resolve skill contexts: %w", err)
	}

	prompt := BuildSystemPrompt(
		basePrompt,
		cwdAgents,
		availableSkills,
		selectedSkills,
		selectedSkillContextFromMeta(meta),
	)
	return appendRuntimeContext(prompt, meta), nil
}

func appendRuntimeContext(prompt string, meta op.Meta) string {
	sections := []string{
		fmt.Sprintf("## Runtime Context\n- current_date: %s", time.Now().Format("2006-01-02")),
	}

	threadID := metaString(meta, "threadID")
	if threadID != "" {
		sections = append(sections, "- threadID: "+threadID)
		sections = append(sections, "- report_dir: reports/"+threadID)
	} else {
		sections = append(sections, "- threadID: unavailable")
		sections = append(sections, "- report_dir: reports/general")
	}

	if chatPath := metaString(meta, "chatPath"); chatPath != "" {
		sections = append(sections, "- chatPath: "+chatPath)
	}
	if cwd := metaString(meta, "cwd"); cwd != "" {
		sections = append(sections, "- cwd: "+cwd)
	}

	appendix := strings.Join(sections, "\n")
	if strings.TrimSpace(prompt) == "" {
		return appendix
	}
	return strings.TrimRight(prompt, "\n") + "\n\n" + appendix
}

func resolveSkillContexts(ctx context.Context, session *op.ServerSession, meta op.Meta) ([]SkillContext, []SkillContext, error) {
	if session == nil {
		return nil, nil, fmt.Errorf("server session is required")
	}
	agentKey := metaString(meta, "key")
	if agentKey == "" {
		return nil, nil, nil
	}

	result, err := session.OpNode(ctx, &op.OpNodeParams{
		OpCode: op.OpNodeList,
		Meta:   op.Meta{"refresh": false},
	})
	if err != nil {
		return nil, nil, err
	}
	jsonContent, ok := result.Content.(*op.JsonContent)
	if !ok {
		return nil, nil, fmt.Errorf("node/list did not return json content")
	}

	var nodes []listedNode
	if err := json.Unmarshal(jsonContent.Raw, &nodes); err != nil {
		return nil, nil, fmt.Errorf("decode node list: %w", err)
	}

	agentSkillKeys := []string(nil)
	skillByKey := make(map[string]SkillContext)
	for _, node := range nodes {
		switch strings.TrimSpace(node.Kind) {
		case string(op.NodeKindAgent):
			if strings.TrimSpace(node.Key) != agentKey {
				continue
			}
			var agentMeta listedAgentMeta
			if err := json.Unmarshal(node.Meta, &agentMeta); err != nil {
				return nil, nil, fmt.Errorf("decode agent meta: %w", err)
			}
			agentSkillKeys = append(agentSkillKeys, agentMeta.Skills...)
		case string(op.NodeKindSkill):
			var skillMeta listedSkillMeta
			if err := json.Unmarshal(node.Meta, &skillMeta); err != nil {
				return nil, nil, fmt.Errorf("decode skill meta: %w", err)
			}
			skillByKey[strings.TrimSpace(node.Key)] = SkillContext{
				Key:         strings.TrimSpace(node.Key),
				Slug:        strings.TrimSpace(skillMeta.Slug),
				Name:        strings.TrimSpace(skillMeta.Name),
				Description: strings.TrimSpace(skillMeta.Description),
				SkillFile:   strings.TrimSpace(op.URIToPath(node.URI)),
				SkillDir:    strings.TrimSpace(node.Cwd),
			}
		}
	}

	selectedKeys := make(map[string]struct{})
	for _, key := range selectedSkillKeysFromMeta(meta) {
		selectedKeys[key] = struct{}{}
	}

	available := make([]SkillContext, 0, len(agentSkillKeys))
	selected := make([]SkillContext, 0, len(selectedKeys))
	selectedSeen := make(map[string]struct{})
	for _, key := range agentSkillKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		skill, ok := skillByKey[key]
		if !ok {
			continue
		}
		if _, isSelected := selectedKeys[key]; isSelected {
			if _, exists := selectedSeen[key]; !exists {
				selected = append(selected, skill)
				selectedSeen[key] = struct{}{}
			}
			continue
		}
		available = append(available, skill)
	}

	for _, key := range selectedSkillKeysFromMeta(meta) {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := selectedSeen[key]; exists {
			continue
		}
		skill, ok := skillByKey[key]
		if !ok {
			continue
		}
		selected = append(selected, skill)
		selectedSeen[key] = struct{}{}
	}

	return available, selected, nil
}

func loadAgentRegistrationMeta() (string, *op.AgentMeta, error) {
	agentFile, err := resolveAgentFileFromExecutable()
	if err != nil {
		return "", nil, err
	}
	data, err := os.ReadFile(agentFile)
	if err != nil {
		return "", nil, err
	}
	frontMatter, ok := extractFrontMatter(string(data))
	if !ok {
		return "", nil, fmt.Errorf("AGENT.md missing YAML front matter")
	}
	var cfg agentConfig
	if err := yaml.Unmarshal([]byte(frontMatter), &cfg); err != nil {
		return "", nil, fmt.Errorf("parse agent front matter: %w", err)
	}
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return "", nil, fmt.Errorf("agent name is required")
	}
	return agentFile, &op.AgentMeta{
		Name:        name,
		Description: strings.TrimSpace(cfg.Description),
	}, nil
}

func resolveAgentFileFromExecutable() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return resolveAgentFileFromExecutablePath(exePath), nil
}

func resolveAgentFileFromExecutablePath(exePath string) string {
	binDir := filepath.Dir(exePath)
	agentDir := filepath.Dir(binDir)
	return filepath.Join(agentDir, ".agent", "AGENT.md")
}

func extractFrontMatter(content string) (string, bool) {
	content = strings.TrimPrefix(content, "\ufeff")
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", false
	}
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" || line == "..." {
			return strings.Join(lines[1:i], "\n"), true
		}
	}
	return "", false
}

func selectedSkillKeysFromMeta(meta op.Meta) []string {
	if meta == nil {
		return nil
	}
	raw := meta["selectedSkillKeys"]
	switch typed := raw.(type) {
	case []string:
		next := make([]string, 0, len(typed))
		for _, value := range typed {
			value = strings.TrimSpace(value)
			if value != "" {
				next = append(next, value)
			}
		}
		return next
	case []any:
		next := make([]string, 0, len(typed))
		for _, value := range typed {
			text, ok := value.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text != "" {
				next = append(next, text)
			}
		}
		return next
	default:
		return nil
	}
}

func selectedSkillContextFromMeta(meta op.Meta) map[string]any {
	if meta == nil {
		return nil
	}
	raw := meta["selectedSkillContext"]
	switch typed := raw.(type) {
	case op.Meta:
		return typed.Clone()
	case map[string]any:
		next := make(map[string]any, len(typed))
		for key, value := range typed {
			next[key] = value
		}
		return next
	default:
		return nil
	}
}

func metaString(meta op.Meta, key string) string {
	if meta == nil {
		return ""
	}
	value, _ := meta[key].(string)
	return strings.TrimSpace(value)
}

func cloneMeta(meta op.Meta) op.Meta {
	if meta == nil {
		return nil
	}
	return meta.Clone()
}
