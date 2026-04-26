package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/op-agent/opagent-protocol/go-sdk/op"
)

func TestResolveSkillContextsUsesNodeIDs(t *testing.T) {
	ctx := context.Background()
	server := op.NewServer(&op.Implementation{Name: "researcher", Version: "v0.0.1"}, nil)

	serverTransport, clientTransport := op.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect(): %v", err)
	}
	defer serverSession.Close()

	nodes := []listedNode{
		{
			ID:   "agent-researcher",
			Kind: string(op.NodeKindAgent),
			Meta: rawJSON(t, listedAgentMeta{Skills: []string{"skill-deep-research", "skill-report"}}),
		},
		{
			ID:   "skill-deep-research",
			Kind: string(op.NodeKindSkill),
			URI:  op.PathToURI("/tmp/opagent/skills/deep-research/SKILL.md"),
			Cwd:  "/tmp/opagent/skills/deep-research",
			Meta: rawJSON(t, listedSkillMeta{Slug: "deep-research", Name: "Deep Research", Description: "Research deeply"}),
		},
		{
			ID:   "skill-report",
			Kind: string(op.NodeKindSkill),
			URI:  op.PathToURI("/tmp/opagent/skills/report/SKILL.md"),
			Cwd:  "/tmp/opagent/skills/report",
			Meta: rawJSON(t, listedSkillMeta{Slug: "report", Name: "Report", Description: "Write a report"}),
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
		"agentID":          "agent-researcher",
		"selectedSkillIDs": []string{"skill-report"},
	})
	if err != nil {
		t.Fatalf("resolveSkillContexts(): %v", err)
	}
	if len(available) != 1 || available[0].ID != "skill-deep-research" {
		t.Fatalf("available = %+v, want only skill-deep-research", available)
	}
	if len(selected) != 1 || selected[0].ID != "skill-report" {
		t.Fatalf("selected = %+v, want only skill-report", selected)
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
