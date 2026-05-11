package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/op-agent/OpAgent/opagent-protocol/go-sdk/op"
	"gopkg.in/yaml.v3"
)

type agentConfig struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type listedNode struct {
	ID   string          `json:"id"`
	Kind string          `json:"kind"`
	URI  string          `json:"uri"`
	Cwd  string          `json:"cwd"`
	Meta json.RawMessage `json:"meta"`
}

type listedAgentMeta struct {
	Name      string   `json:"name"`
	Skills    []string `json:"skills"`
	SubAgents []string `json:"subAgents"`
}

type listedSkillMeta struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type systemConfigPayload struct {
	BaseDir string `json:"baseDir"`
}

var defaultMemoryUpdates = newMemoryUpdateScheduler(8, 3*time.Minute)
var enqueueMemoryUpdate = defaultMemoryUpdates.Enqueue

func main() {
	agentFile, meta, err := loadAgentRegistrationMeta()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load opagent agent metadata: %v\n", err)
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
	defaultMemoryUpdates.Start(ctx)

	if err := server.Run(ctx, &op.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "opagent agent server failed: %v\n", err)
		os.Exit(1)
	}
}

func handleCallAgent(ctx context.Context, req *op.CallAgentRequest) (*op.CallAgentResult, error) {
	if req == nil || req.Session == nil || req.Params == nil {
		return nil, fmt.Errorf("agent call params are required")
	}
	parentMeta := cloneMeta(req.Params.Meta)
	result, err := req.Session.OpNode(ctx, &op.OpNodeParams{
		OpCode:  op.OpAgentLoopCreate,
		Meta:    parentMeta,
		Content: req.Params.Content,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("agent loop returned nil result")
	}
	enqueueMemoryUpdate(memoryUpdateJob{
		Session:       req.Session,
		ParentAgentID: metaString(parentMeta, "agentID"),
		ParentMeta:    parentMeta,
		ParentContent: req.Params.Content,
		ResultMeta:    cloneMeta(result.Meta),
		ResultContent: result.Content,
	})
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
	platform := runtime.GOOS
	basePrompt = ExpandPlatformVariables(basePrompt, platform)

	cwdAgentsPath, err := ResolveCwdAgentsPath(metaString(meta, "cwd"))
	if err != nil {
		return "", fmt.Errorf("resolve cwd AGENTS.md: %w", err)
	}

	availableSkills, selectedSkills, err := resolveSkillContexts(ctx, session, meta)
	if err != nil {
		return "", fmt.Errorf("resolve skill contexts: %w", err)
	}

	memoryPath := resolveOpAgentMemoryPath(ctx, session)
	return BuildSystemPromptWithPaths(basePrompt, ResolveOpAgentShellContext(platform), cwdAgentsPath, memoryPath, availableSkills, selectedSkills, selectedSkillContextFromMeta(meta)), nil
}

func resolveOpAgentMemoryPath(ctx context.Context, session *op.ServerSession) string {
	baseDir, err := resolveSystemBaseDir(ctx, session)
	if err != nil {
		slog.Debug("opagent memory path unavailable", "error", err)
		return ""
	}
	return filepath.Join(baseDir, "agents", "opagent", "memory.md")
}

func resolveSystemBaseDir(ctx context.Context, session *op.ServerSession) (string, error) {
	if session == nil {
		return "", fmt.Errorf("server session is required")
	}
	result, err := session.OpNode(ctx, &op.OpNodeParams{OpCode: op.ConfigSystemGet})
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", fmt.Errorf("config/system/get returned nil result")
	}
	jsonContent, ok := result.Content.(*op.JsonContent)
	if !ok || jsonContent == nil {
		return "", fmt.Errorf("config/system/get did not return json content")
	}
	var cfg systemConfigPayload
	if err := json.Unmarshal(jsonContent.Raw, &cfg); err != nil {
		return "", fmt.Errorf("decode system config: %w", err)
	}
	baseDir := strings.TrimSpace(cfg.BaseDir)
	if baseDir == "" {
		return "", fmt.Errorf("system baseDir is required")
	}
	return baseDir, nil
}

func loadListedNodes(ctx context.Context, session *op.ServerSession) ([]listedNode, error) {
	if session == nil {
		return nil, fmt.Errorf("server session is required")
	}
	result, err := session.OpNode(ctx, &op.OpNodeParams{
		OpCode: op.OpNodeList,
		Meta:   op.Meta{"refresh": false},
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("node/list returned nil result")
	}
	jsonContent, ok := result.Content.(*op.JsonContent)
	if !ok || jsonContent == nil {
		return nil, fmt.Errorf("node/list did not return json content")
	}

	var nodes []listedNode
	if err := json.Unmarshal(jsonContent.Raw, &nodes); err != nil {
		return nil, fmt.Errorf("decode node list: %w", err)
	}
	return nodes, nil
}

func resolveSkillContexts(ctx context.Context, session *op.ServerSession, meta op.Meta) ([]SkillContext, []SkillContext, error) {
	if session == nil {
		return nil, nil, fmt.Errorf("server session is required")
	}
	agentID := metaString(meta, "agentID")
	if agentID == "" {
		return nil, nil, nil
	}

	nodes, err := loadListedNodes(ctx, session)
	if err != nil {
		return nil, nil, err
	}

	agentSkillIDs := []string(nil)
	skillByID := make(map[string]SkillContext)
	for _, node := range nodes {
		switch strings.TrimSpace(node.Kind) {
		case string(op.NodeKindAgent):
			if strings.TrimSpace(node.ID) != agentID {
				continue
			}
			var agentMeta listedAgentMeta
			if err := json.Unmarshal(node.Meta, &agentMeta); err != nil {
				return nil, nil, fmt.Errorf("decode agent meta: %w", err)
			}
			agentSkillIDs = append(agentSkillIDs, agentMeta.Skills...)
		case string(op.NodeKindSkill):
			var skillMeta listedSkillMeta
			if err := json.Unmarshal(node.Meta, &skillMeta); err != nil {
				return nil, nil, fmt.Errorf("decode skill meta: %w", err)
			}
			skillID := strings.TrimSpace(node.ID)
			skillByID[skillID] = SkillContext{
				ID:          skillID,
				Slug:        strings.TrimSpace(skillMeta.Slug),
				Name:        strings.TrimSpace(skillMeta.Name),
				Description: strings.TrimSpace(skillMeta.Description),
				SkillFile:   strings.TrimSpace(op.URIToPath(node.URI)),
				SkillDir:    strings.TrimSpace(node.Cwd),
			}
		}
	}

	selectedIDs := make(map[string]struct{})
	for _, id := range selectedSkillIDsFromMeta(meta) {
		selectedIDs[id] = struct{}{}
	}

	available := make([]SkillContext, 0, len(agentSkillIDs))
	selected := make([]SkillContext, 0, len(selectedIDs))
	selectedSeen := make(map[string]struct{})
	for _, id := range agentSkillIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		skill, ok := skillByID[id]
		if !ok {
			continue
		}
		if _, isSelected := selectedIDs[id]; isSelected {
			if _, exists := selectedSeen[id]; !exists {
				selected = append(selected, skill)
				selectedSeen[id] = struct{}{}
			}
			continue
		}
		available = append(available, skill)
	}

	for _, id := range selectedSkillIDsFromMeta(meta) {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := selectedSeen[id]; exists {
			continue
		}
		skill, ok := skillByID[id]
		if !ok {
			continue
		}
		selected = append(selected, skill)
		selectedSeen[id] = struct{}{}
	}

	return available, selected, nil
}

type memoryUpdateJob struct {
	Session       *op.ServerSession
	ParentAgentID string
	ParentMeta    op.Meta
	ParentContent op.Content
	ResultMeta    op.Meta
	ResultContent op.Content
}

type memoryUpdateScheduler struct {
	jobs    chan memoryUpdateJob
	timeout time.Duration
	once    sync.Once
}

func newMemoryUpdateScheduler(capacity int, timeout time.Duration) *memoryUpdateScheduler {
	if capacity <= 0 {
		capacity = 1
	}
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	return &memoryUpdateScheduler{
		jobs:    make(chan memoryUpdateJob, capacity),
		timeout: timeout,
	}
}

func (s *memoryUpdateScheduler) Start(ctx context.Context) {
	if s == nil {
		return
	}
	s.once.Do(func() {
		go s.run(ctx)
	})
}

func (s *memoryUpdateScheduler) Enqueue(job memoryUpdateJob) bool {
	if s == nil || job.Session == nil {
		return false
	}
	select {
	case s.jobs <- job:
		return true
	default:
		slog.Warn("opagent memory update queue full; skipping update",
			"threadID", firstNonEmpty(metaString(job.ResultMeta, "threadID"), metaString(job.ParentMeta, "threadID")),
			"turnID", metaString(job.ResultMeta, "turnID"),
		)
		return false
	}
}

func (s *memoryUpdateScheduler) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.jobs:
			jobCtx, cancel := context.WithTimeout(context.Background(), s.timeout)
			if err := runMemoryUpdate(jobCtx, job); err != nil {
				slog.Warn("opagent memory update failed", "error", err)
			}
			cancel()
		}
	}
}

type memoryTurnSummary struct {
	MemoryPath     string
	ParentChatPath string
	ParentThreadID string
	ParentTurnID   string
	ModelKey       string
	UserText       string
	AssistantText  string
}

func runMemoryUpdate(ctx context.Context, job memoryUpdateJob) error {
	if job.Session == nil {
		return fmt.Errorf("server session is required")
	}
	baseDir, err := resolveSystemBaseDir(ctx, job.Session)
	if err != nil {
		return fmt.Errorf("resolve baseDir: %w", err)
	}
	nodes, err := loadListedNodes(ctx, job.Session)
	if err != nil {
		return fmt.Errorf("load nodes: %w", err)
	}
	memoryNode, err := resolveMemorySubagentNode(firstNonEmpty(job.ParentAgentID, metaString(job.ParentMeta, "agentID"), metaString(job.ResultMeta, "agentID")), nodes)
	if err != nil {
		return err
	}

	summary := buildMemoryTurnSummary(baseDir, job)
	if summary.ModelKey == "" {
		return fmt.Errorf("memory update requires parent modelKey")
	}
	sessionResult, err := createMemoryChatSession(ctx, job.Session, memoryNode, baseDir, summary)
	if err != nil {
		return fmt.Errorf("create memory session: %w", err)
	}
	return runMemoryUpdateLoop(ctx, job.Session, memoryNode, sessionResult, summary)
}

func resolveMemorySubagentNode(parentAgentID string, nodes []listedNode) (*listedNode, error) {
	agentByID := make(map[string]listedNode, len(nodes))
	for _, node := range nodes {
		if strings.TrimSpace(node.Kind) != string(op.NodeKindAgent) {
			continue
		}
		id := strings.TrimSpace(node.ID)
		if id == "" {
			continue
		}
		agentByID[id] = node
	}

	parent, ok := agentByID[strings.TrimSpace(parentAgentID)]
	if ok {
		var parentMeta listedAgentMeta
		if err := json.Unmarshal(parent.Meta, &parentMeta); err != nil {
			return nil, fmt.Errorf("decode parent agent meta: %w", err)
		}
		var firstSubagent *listedNode
		for _, id := range parentMeta.SubAgents {
			node, exists := agentByID[strings.TrimSpace(id)]
			if !exists {
				continue
			}
			nodeCopy := node
			if firstSubagent == nil {
				firstSubagent = &nodeCopy
			}
			var childMeta listedAgentMeta
			if err := json.Unmarshal(node.Meta, &childMeta); err != nil {
				return nil, fmt.Errorf("decode subagent meta: %w", err)
			}
			if strings.TrimSpace(childMeta.Name) == "opagent-memory" {
				return &nodeCopy, nil
			}
		}
		if firstSubagent != nil && len(parentMeta.SubAgents) == 1 {
			return firstSubagent, nil
		}
	}

	for _, node := range agentByID {
		var meta listedAgentMeta
		if err := json.Unmarshal(node.Meta, &meta); err != nil {
			return nil, fmt.Errorf("decode agent meta: %w", err)
		}
		if strings.TrimSpace(meta.Name) == "opagent-memory" {
			nodeCopy := node
			return &nodeCopy, nil
		}
	}
	return nil, fmt.Errorf("opagent-memory subagent not found")
}

func buildMemoryTurnSummary(baseDir string, job memoryUpdateJob) memoryTurnSummary {
	parentThreadID := firstNonEmpty(metaString(job.ResultMeta, "threadID"), metaString(job.ParentMeta, "threadID"))
	parentTurnID := firstNonEmpty(metaString(job.ResultMeta, "turnID"), metaString(job.ParentMeta, "turnID"))
	parentChatPath := firstNonEmpty(metaString(job.ResultMeta, "chatPath"), metaString(job.ResultMeta, "path"), metaString(job.ParentMeta, "chatPath"), metaString(job.ParentMeta, "path"))
	return memoryTurnSummary{
		MemoryPath:     filepath.Join(baseDir, "agents", "opagent", "memory.md"),
		ParentChatPath: parentChatPath,
		ParentThreadID: parentThreadID,
		ParentTurnID:   parentTurnID,
		ModelKey:       firstNonEmpty(metaString(job.ResultMeta, "modelKey"), metaString(job.ParentMeta, "modelKey")),
		UserText:       truncateForPrompt(userTextFromContent(job.ParentContent), 4000),
		AssistantText:  truncateForPrompt(assistantTextFromResult(job.ResultContent), 6000),
	}
}

func createMemoryChatSession(ctx context.Context, session *op.ServerSession, memoryNode *listedNode, baseDir string, summary memoryTurnSummary) (*op.ChatSessionCreateResult, error) {
	if memoryNode == nil {
		return nil, fmt.Errorf("memory node is required")
	}
	memoryCWD := filepath.Join(baseDir, "agents", "opagent")
	chatDir := filepath.Join(memoryCWD, ".agent", "chat", "memory")
	if err := os.MkdirAll(chatDir, 0o755); err != nil {
		return nil, err
	}
	name := sanitizePathPart(firstNonEmpty(summary.ParentThreadID, "thread")) + "-" + sanitizePathPart(firstNonEmpty(summary.ParentTurnID, fmt.Sprintf("%d", time.Now().UnixNano()))) + ".md"
	params := op.ChatSessionCreateParams{
		AgentID:  strings.TrimSpace(memoryNode.ID),
		CWD:      memoryCWD,
		ChatPath: filepath.Join(chatDir, name),
		Title:    "Memory update",
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	result, err := session.OpNode(ctx, &op.OpNodeParams{
		OpCode:  op.OpChatSessionCreate,
		Meta:    op.Meta{"agentID": params.AgentID},
		Content: op.NewJsonContentRaw(raw),
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("chat/session/create returned nil result")
	}
	jsonContent, ok := result.Content.(*op.JsonContent)
	if !ok || jsonContent == nil {
		return nil, fmt.Errorf("chat/session/create did not return json content")
	}
	var created op.ChatSessionCreateResult
	if err := json.Unmarshal(jsonContent.Raw, &created); err != nil {
		return nil, fmt.Errorf("decode chat session create result: %w", err)
	}
	return &created, nil
}

func runMemoryUpdateLoop(ctx context.Context, session *op.ServerSession, memoryNode *listedNode, created *op.ChatSessionCreateResult, summary memoryTurnSummary) error {
	if memoryNode == nil || created == nil {
		return fmt.Errorf("memory node and session are required")
	}
	memoryCWD := filepath.Dir(strings.TrimSpace(summary.MemoryPath))
	chatPath := firstNonEmpty(created.ChatPath, created.Path)
	meta := op.Meta{
		"agentID":       strings.TrimSpace(memoryNode.ID),
		"threadID":      strings.TrimSpace(created.ThreadID),
		"cwd":           memoryCWD,
		"chatPath":      chatPath,
		"path":          chatPath,
		"title":         firstNonEmpty(created.Title, "Memory update"),
		"modelKey":      summary.ModelKey,
		"thinkingLevel": "off",
	}
	if summary.ParentThreadID != "" {
		meta["parentThreadID"] = summary.ParentThreadID
	}
	_, err := session.OpNode(ctx, &op.OpNodeParams{
		OpCode:  op.OpAgentLoopCreate,
		Meta:    meta,
		Content: &op.TextContent{Text: buildMemoryUpdatePrompt(summary)},
	})
	return err
}

func buildMemoryUpdatePrompt(summary memoryTurnSummary) string {
	lines := []string{
		"Update the durable OpAgent memory file.",
		"",
		"Paths and source context:",
		"- memoryPath: " + summary.MemoryPath,
		"- parentChatPath: " + firstNonEmpty(summary.ParentChatPath, "unavailable"),
		"- parentThreadID: " + firstNonEmpty(summary.ParentThreadID, "unavailable"),
		"- parentTurnID: " + firstNonEmpty(summary.ParentTurnID, "unavailable"),
		"",
		"Use the read tool to inspect parentChatPath when it is available. If the markdown file has not yet caught up with this turn, use the turn excerpt below as the source for this update.",
		"",
		"Parent user message:",
		fencedForPrompt(summary.UserText),
		"",
		"Parent assistant response:",
		fencedForPrompt(summary.AssistantText),
	}
	return strings.Join(lines, "\n")
}

func userTextFromContent(content op.Content) string {
	msg, err := op.DecodeUserMessageContent(content)
	if err != nil {
		if text, ok := content.(*op.TextContent); ok && text != nil {
			return strings.TrimSpace(text.Text)
		}
		return ""
	}
	return messageText(msg)
}

func assistantTextFromResult(content op.Content) string {
	switch typed := content.(type) {
	case *op.JsonContent:
		if typed == nil {
			return ""
		}
		var messages []op.Message
		if err := json.Unmarshal(typed.Raw, &messages); err != nil {
			return ""
		}
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role != op.RoleAssistant {
				continue
			}
			if text := messageText(messages[i]); text != "" {
				return text
			}
		}
	case *op.TextContent:
		if typed != nil {
			return strings.TrimSpace(typed.Text)
		}
	}
	return ""
}

func messageText(msg op.Message) string {
	if text := strings.TrimSpace(msg.Content); text != "" {
		return text
	}
	parts := make([]string, 0, len(msg.ContentParts))
	for _, part := range msg.ContentParts {
		if text := strings.TrimSpace(part.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func truncateForPrompt(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return strings.TrimSpace(text[:limit]) + "\n[truncated]"
}

func fencedForPrompt(text string) string {
	text = strings.ReplaceAll(strings.TrimSpace(text), "```", "` ` `")
	if text == "" {
		text = "unavailable"
	}
	return "```\n" + text + "\n```"
}

func sanitizePathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	if len(out) > 96 {
		out = out[:96]
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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

func selectedSkillIDsFromMeta(meta op.Meta) []string {
	if meta == nil {
		return nil
	}
	raw := meta["selectedSkillIDs"]
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
