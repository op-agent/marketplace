package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/op-agent/opagent-dev/packages/opagent-protocol-sdk-go/op"
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
}
