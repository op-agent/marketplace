package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/op-agent/opagent-protocol/go-sdk/op"
)

func TestResolveAgentFileFromExecutablePath(t *testing.T) {
	exePath := filepath.Join("/tmp", "opagent", "bin", "opagent")
	got := resolveAgentFileFromExecutablePath(exePath)
	want := filepath.Join("/tmp", "opagent", ".agent", "AGENT.md")
	if got != want {
		t.Fatalf("resolveAgentFileFromExecutablePath() = %q, want %q", got, want)
	}
}

func TestHandleCallAgent_ForwardsToOpAgentLoopCreate(t *testing.T) {
	ctx := context.Background()
	oldEnqueue := enqueueMemoryUpdate
	enqueued := false
	enqueueMemoryUpdate = func(job memoryUpdateJob) bool {
		enqueued = true
		if job.Session == nil {
			t.Fatal("memory job session = nil")
		}
		if got := metaString(job.ParentMeta, "threadID"); got != "thread-test" {
			t.Fatalf("memory job parent threadID = %q, want thread-test", got)
		}
		return true
	}
	defer func() { enqueueMemoryUpdate = oldEnqueue }()

	server := op.NewServer(&op.Implementation{Name: "opagent", Version: "v0.0.1"}, nil)
	server.AddAgent(&op.AgentMeta{Name: "opagent"}, handleCallAgent)

	serverTransport, clientTransport := op.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	defer serverSession.Close()

	var captured *op.OpNodeParams
	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, &op.ClientOptions{
		OpNodeHandler: func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			if req == nil || req.Params == nil {
				t.Fatal("OpNodeHandler received nil params")
			}
			clone := *req.Params
			clone.Meta = cloneMeta(req.Params.Meta)
			captured = &clone
			return &op.OpNodeResult{
				OpCode:  req.Params.OpCode,
				Meta:    op.Meta{"forwarded": true},
				Content: &op.TextContent{Text: "ok"},
			}, nil
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}
	defer clientSession.Close()

	originalMeta := op.Meta{"threadID": "thread-test", "chatPath": "/tmp/chat.md"}
	result, err := clientSession.CallAgent(ctx, &op.CallAgentParams{
		AgentID: "opagent",
		Meta:    originalMeta,
		Content: &op.TextContent{Text: "hello"},
	})
	if err != nil {
		t.Fatalf("CallAgent(): %v", err)
	}
	if captured == nil {
		t.Fatal("captured OpNode params = nil")
	}
	if captured.OpCode != op.OpAgentLoopCreate {
		t.Fatalf("forwarded OpCode = %q, want %q", captured.OpCode, op.OpAgentLoopCreate)
	}
	if got := captured.Meta["opcode"]; got != nil {
		t.Fatalf("forwarded meta opcode = %#v, want nil", got)
	}
	if got := originalMeta["opcode"]; got != nil {
		t.Fatalf("original meta opcode = %#v, want nil", got)
	}
	content, ok := captured.Content.(*op.TextContent)
	if !ok {
		t.Fatalf("forwarded content type = %T, want *op.TextContent", captured.Content)
	}
	if content.Text != "hello" {
		t.Fatalf("forwarded content = %q, want hello", content.Text)
	}
	if result == nil {
		t.Fatal("CallAgent() result = nil")
	}
	if result.AgentID != "opagent" {
		t.Fatalf("result.AgentID = %q, want opagent", result.AgentID)
	}
	if got, _ := result.Meta["forwarded"].(bool); !got {
		t.Fatalf("result.Meta[forwarded] = %#v, want true", result.Meta["forwarded"])
	}
	if !enqueued {
		t.Fatal("memory update was not enqueued")
	}
}

func TestHandleCallAgent_DoesNotEnqueueMemoryOnForwardError(t *testing.T) {
	ctx := context.Background()
	oldEnqueue := enqueueMemoryUpdate
	enqueued := false
	enqueueMemoryUpdate = func(job memoryUpdateJob) bool {
		enqueued = true
		return true
	}
	defer func() { enqueueMemoryUpdate = oldEnqueue }()

	server := op.NewServer(&op.Implementation{Name: "opagent", Version: "v0.0.1"}, nil)
	server.AddAgent(&op.AgentMeta{Name: "opagent"}, handleCallAgent)

	serverTransport, clientTransport := op.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	defer serverSession.Close()

	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, &op.ClientOptions{
		OpNodeHandler: func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			return nil, fmt.Errorf("forward failed")
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}
	defer clientSession.Close()

	_, err = clientSession.CallAgent(ctx, &op.CallAgentParams{
		AgentID: "opagent",
		Meta:    op.Meta{"threadID": "thread-test"},
		Content: &op.TextContent{Text: "hello"},
	})
	if err == nil {
		t.Fatal("CallAgent() succeeded, want error")
	}
	if enqueued {
		t.Fatal("memory update was enqueued after failed parent turn")
	}
}

func TestBuildPromptIncludesMemoryPath(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	agentFile := filepath.Join(tempDir, "AGENT.md")
	if err := os.WriteFile(agentFile, []byte("---\nname: opagent\n---\nBase prompt"), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}
	baseDir := filepath.Join(tempDir, "base")

	server := op.NewServer(&op.Implementation{Name: "opagent", Version: "v0.0.1"}, nil)
	serverTransport, clientTransport := op.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	defer serverSession.Close()

	nodes := []listedNode{{
		ID:   "agent-opagent",
		Kind: string(op.NodeKindAgent),
		Meta: rawJSON(t, listedAgentMeta{Name: "opagent"}),
	}}
	rawNodes, err := json.Marshal(nodes)
	if err != nil {
		t.Fatalf("marshal nodes: %v", err)
	}

	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, &op.ClientOptions{
		OpNodeHandler: func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			switch req.Params.OpCode {
			case op.OpNodeList:
				return &op.OpNodeResult{Content: &op.JsonContent{Raw: rawNodes}}, nil
			case op.ConfigSystemGet:
				return &op.OpNodeResult{Content: rawJSONContent(t, systemConfigPayload{BaseDir: baseDir})}, nil
			default:
				return nil, fmt.Errorf("unexpected opcode: %s", req.Params.OpCode)
			}
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}
	defer clientSession.Close()

	prompt, err := buildPrompt(ctx, serverSession, agentFile, op.Meta{"agentID": "agent-opagent"})
	if err != nil {
		t.Fatalf("buildPrompt(): %v", err)
	}
	want := filepath.Join(baseDir, "agents", "opagent", "memory.md")
	if !strings.Contains(prompt, want) {
		t.Fatalf("prompt missing memory path %q:\n%s", want, prompt)
	}
}

func TestResolveSkillContextsUsesNodeIDs(t *testing.T) {
	ctx := context.Background()
	server := op.NewServer(&op.Implementation{Name: "opagent", Version: "v0.0.1"}, nil)

	serverTransport, clientTransport := op.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	defer serverSession.Close()

	nodes := []listedNode{
		{
			ID:   "agent-alpha",
			Kind: string(op.NodeKindAgent),
			Meta: rawJSON(t, listedAgentMeta{Skills: []string{"skill-plan", "skill-execute"}}),
		},
		{
			ID:   "skill-plan",
			Kind: string(op.NodeKindSkill),
			URI:  op.PathToURI("/tmp/opagent/skills/plan/SKILL.md"),
			Cwd:  "/tmp/opagent/skills/plan",
			Meta: rawJSON(t, listedSkillMeta{Slug: "plan", Name: "Plan", Description: "Make a plan"}),
		},
		{
			ID:   "skill-execute",
			Kind: string(op.NodeKindSkill),
			URI:  op.PathToURI("/tmp/opagent/skills/execute-plan/SKILL.md"),
			Cwd:  "/tmp/opagent/skills/execute-plan",
			Meta: rawJSON(t, listedSkillMeta{Slug: "execute-plan", Name: "Execute Plan", Description: "Execute a plan"}),
		},
	}
	rawNodes, err := json.Marshal(nodes)
	if err != nil {
		t.Fatalf("marshal nodes: %v", err)
	}

	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, &op.ClientOptions{
		OpNodeHandler: func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			if req == nil || req.Params == nil {
				t.Fatal("OpNodeHandler received nil params")
			}
			if req.Params.OpCode != op.OpNodeList {
				t.Fatalf("OpCode = %q, want %q", req.Params.OpCode, op.OpNodeList)
			}
			return &op.OpNodeResult{Content: &op.JsonContent{Raw: rawNodes}}, nil
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}
	defer clientSession.Close()

	available, selected, err := resolveSkillContexts(ctx, serverSession, op.Meta{
		"agentID":          "agent-alpha",
		"selectedSkillIDs": []any{"skill-execute"},
	})
	if err != nil {
		t.Fatalf("resolveSkillContexts(): %v", err)
	}
	if len(available) != 1 || available[0].ID != "skill-plan" {
		t.Fatalf("available = %+v, want only skill-plan", available)
	}
	if len(selected) != 1 || selected[0].ID != "skill-execute" {
		t.Fatalf("selected = %+v, want only skill-execute", selected)
	}
}

func TestRunMemoryUpdateRunsMemoryAgentLoopWithInheritedModel(t *testing.T) {
	ctx := context.Background()
	baseDir := t.TempDir()
	parentID := "agent-parent"
	childID := "agent-memory"
	nodes := []listedNode{
		{
			ID:   parentID,
			Kind: string(op.NodeKindAgent),
			Meta: rawJSON(t, listedAgentMeta{Name: "opagent", SubAgents: []string{childID}}),
		},
		{
			ID:   childID,
			Kind: string(op.NodeKindAgent),
			URI:  op.PathToURI(filepath.Join(baseDir, "agents", "opagent", "subagents", "memory", ".agent", "AGENT.md")),
			Cwd:  filepath.Join(baseDir, "agents", "opagent", "subagents", "memory"),
			Meta: rawJSON(t, listedAgentMeta{Name: "opagent-memory"}),
		},
	}
	rawNodes, err := json.Marshal(nodes)
	if err != nil {
		t.Fatalf("marshal nodes: %v", err)
	}
	parentMessages, err := json.Marshal([]op.Message{{Role: op.RoleAssistant, Content: "Implemented the feature."}})
	if err != nil {
		t.Fatalf("marshal parent messages: %v", err)
	}

	server := op.NewServer(&op.Implementation{Name: "opagent", Version: "v0.0.1"}, nil)
	serverTransport, clientTransport := op.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	defer serverSession.Close()

	var loopMeta op.Meta
	var loopText string
	client := op.NewClient(&op.Implementation{Name: "client", Version: "v0.0.1"}, &op.ClientOptions{
		OpNodeHandler: func(_ context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
			switch req.Params.OpCode {
			case op.ConfigSystemGet:
				return &op.OpNodeResult{Content: rawJSONContent(t, systemConfigPayload{BaseDir: baseDir})}, nil
			case op.OpNodeList:
				return &op.OpNodeResult{Content: &op.JsonContent{Raw: rawNodes}}, nil
			case op.OpChatSessionCreate:
				var params op.ChatSessionCreateParams
				if err := req.Params.Content.(*op.JsonContent).Unmarshal(&params); err != nil {
					t.Fatalf("decode create params: %v", err)
				}
				if params.AgentID != childID {
					t.Fatalf("create AgentID = %q, want %q", params.AgentID, childID)
				}
				return &op.OpNodeResult{Content: rawJSONContent(t, op.ChatSessionCreateResult{
					ThreadID: "thread-memory",
					Title:    params.Title,
					ChatPath: params.ChatPath,
					Path:     params.ChatPath,
				})}, nil
			case op.OpAgentLoopCreate:
				loopMeta = cloneMeta(req.Params.Meta)
				if text, ok := req.Params.Content.(*op.TextContent); ok && text != nil {
					loopText = text.Text
				}
				return &op.OpNodeResult{Content: &op.TextContent{Text: "ok"}}, nil
			default:
				return nil, fmt.Errorf("unexpected opcode: %s", req.Params.OpCode)
			}
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect(): %v", err)
	}
	defer clientSession.Close()

	err = runMemoryUpdate(ctx, memoryUpdateJob{
		Session:       serverSession,
		ParentAgentID: parentID,
		ParentMeta: op.Meta{
			"threadID": "thread-parent",
			"chatPath": filepath.Join(baseDir, "agents", "opagent", ".agent", "chat", "parent.md"),
			"modelKey": "opagent:gpt-5.4",
		},
		ParentContent: &op.TextContent{Text: "Please implement the feature."},
		ResultMeta:    op.Meta{"threadID": "thread-parent", "turnID": "turn-parent"},
		ResultContent: op.NewJsonContentRaw(parentMessages),
	})
	if err != nil {
		t.Fatalf("runMemoryUpdate(): %v", err)
	}
	if got := metaString(loopMeta, "agentID"); got != childID {
		t.Fatalf("loop agentID = %q, want %q", got, childID)
	}
	if got := metaString(loopMeta, "modelKey"); got != "opagent:gpt-5.4" {
		t.Fatalf("loop modelKey = %q, want opagent:gpt-5.4", got)
	}
	if got := metaString(loopMeta, "thinkingLevel"); got != "off" {
		t.Fatalf("loop thinkingLevel = %q, want off", got)
	}
	if got := metaString(loopMeta, "cwd"); got != filepath.Join(baseDir, "agents", "opagent") {
		t.Fatalf("loop cwd = %q, want opagent agent dir", got)
	}
	if !strings.Contains(loopText, filepath.Join(baseDir, "agents", "opagent", "memory.md")) ||
		!strings.Contains(loopText, "Please implement the feature.") ||
		!strings.Contains(loopText, "Implemented the feature.") {
		t.Fatalf("loop prompt missing memory context:\n%s", loopText)
	}
}

func rawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal raw json: %v", err)
	}
	return raw
}

func rawJSONContent(t *testing.T, value any) *op.JsonContent {
	t.Helper()
	return op.NewJsonContentRaw(rawJSON(t, value))
}
