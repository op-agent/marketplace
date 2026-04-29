package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/op-agent/opagent-protocol/go-sdk/op"
)

const agentID = "claude-code"

func main() {
	agentFile, err := resolveAgentFile()
	if err != nil {
		log.Fatalf("resolve agent file: %v", err)
	}

	meta, err := loadAgentMeta(agentFile)
	if err != nil {
		log.Fatalf("load agent metadata: %v", err)
	}

	server := op.NewServer(&op.Implementation{Name: meta.Name, Version: "0.1.0"}, &op.ServerOptions{
		OpNodeHandler: func(ctx context.Context, req *op.ServerRequest[*op.OpNodeParams]) (*op.OpNodeResult, error) {
			return handleOpNode(ctx, req, agentFile)
		},
	})
	server.AddAgent(meta, func(ctx context.Context, req *op.CallAgentRequest) (*op.CallAgentResult, error) {
		return handleCallAgent(ctx, req, agentFile)
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer stop()

	if err := server.Run(ctx, &op.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("server error: %v", err)
	}
}

func handleOpNode(ctx context.Context, req *op.ServerRequest[*op.OpNodeParams], agentFile string) (*op.OpNodeResult, error) {
	if req == nil || req.Params == nil {
		return nil, fmt.Errorf("node operation params are required")
	}
	if req.Params.OpCode != op.OpPromptGet {
		return nil, fmt.Errorf("unsupported opcode for %s: %s", agentID, req.Params.OpCode)
	}

	body, err := loadAgentPrompt(agentFile)
	if err != nil {
		return nil, err
	}
	return &op.OpNodeResult{
		OpCode:  req.Params.OpCode,
		Meta:    req.Params.Meta.Add(op.Meta{"agentID": agentID}),
		Content: &op.TextContent{Text: body},
	}, nil
}

func handleCallAgent(ctx context.Context, req *op.CallAgentRequest, agentFile string) (*op.CallAgentResult, error) {
	if req == nil || req.Session == nil || req.Params == nil {
		return nil, fmt.Errorf("agent call params are required")
	}

	cfg := claudeConfigFromEnv(lookupEnv)
	if cfg.BridgeMode != "" && !strings.EqualFold(cfg.BridgeMode, "cli") {
		return nil, fmt.Errorf("unsupported Claude Code bridge mode %q: marketplace MVP currently packages CLI bridge only", cfg.BridgeMode)
	}

	prompt, err := promptFromContent(req.Params.Content)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("empty prompt")
	}

	agentPrompt := ""
	if cfg.AppendOpAgentPrompt {
		loaded, err := loadAgentPrompt(agentFile)
		if err != nil {
			return nil, err
		}
		agentPrompt = loaded
	}

	runCtx := ctx
	cancel := func() {}
	if cfg.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	}
	defer cancel()

	baseMeta := req.Params.Meta.Clone().Add(op.Meta{"agentID": agentID})
	result, err := runClaudeCLI(runCtx, cfg, claudeRunInput{
		Prompt:      prompt,
		AgentPrompt: agentPrompt,
		CWD:         cwdFromMeta(req.Params.Meta),
		BaseMeta:    baseMeta,
		Notify: func(ctx context.Context, message string, meta op.Meta) error {
			return notifyText(ctx, req.Session, message, meta)
		},
	})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("Claude Code reported an error: %s", firstNonEmpty(result.ErrorText, result.FinalText, "unknown error"))
	}

	finalText := strings.TrimSpace(firstNonEmpty(result.FinalText, result.AssistantText, result.PlainText))
	if finalText == "" {
		finalText = "Claude Code completed without text output."
	}

	return &op.CallAgentResult{
		AgentID: req.Params.AgentID,
		Meta: baseMeta.Add(op.Meta{
			"claudeCode": map[string]any{
				"sessionID": result.SessionID,
				"model":     result.Model,
				"cwd":       result.CWD,
			},
		}),
		Content: &op.TextContent{Text: finalText},
	}, nil
}

func notifyText(ctx context.Context, session *op.ServerSession, message string, meta op.Meta) error {
	if session == nil || strings.TrimSpace(message) == "" {
		return nil
	}
	return session.NotifyInfo(ctx, &op.InfoNotificationParams{
		OpCode:  op.NotifyMessage,
		Meta:    meta,
		Content: &op.TextContent{Text: message},
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
