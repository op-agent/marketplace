import { query } from "@anthropic-ai/claude-agent-sdk";
import {
  cloneMeta,
  contentText,
  metaString,
  textContent,
  type CallAgentResult,
  type Content,
  type Meta,
  type ServerRequest,
  type CallAgentParams,
} from "@op-agent/opagent-protocol";
import { buildClaudeOptions, type ClaudeCodeConfig } from "./config.js";
import {
  createClaudeRunState,
  finalizeClaudeRun,
  handleClaudeMessage,
} from "./claudeEvents.js";

const sessionByThread = new Map<string, string>();

export async function runClaudeAgent(
  req: ServerRequest<CallAgentParams>,
  cfg: ClaudeCodeConfig,
  agentPrompt: string,
): Promise<CallAgentResult> {
  const prompt = contentText(req.params.content);
  if (!prompt.trim()) {
    throw new Error("empty prompt");
  }

  const baseMeta = cloneMeta(req.params._meta);
  if (!metaString(baseMeta, "agentID")) {
    baseMeta.agentID = req.params.agentID;
  }
  if (cfg.notifyRawEvents) {
    baseMeta.claudeCodeNotifyRawEvents = true;
  }

  const abortController = new AbortController();
  const timeout = cfg.timeoutMs
    ? setTimeout(() => abortController.abort(), cfg.timeoutMs)
    : undefined;

  try {
    const resumeKey = sessionKey(baseMeta);
    const resume = cfg.resumeSessions
      ? firstNonEmpty(metaString(baseMeta, "claudeCodeSessionID"), sessionByThread.get(resumeKey) || "")
      : "";
    const options = await buildClaudeOptions(cfg, {
      cwd: cwdFromMeta(baseMeta),
      agentPrompt,
      resume: resume || undefined,
      abortController,
    });

    const state = createClaudeRunState();
    const stream = query({ prompt, options });
    const notify = (content: Content, meta: Meta) => req.session.notifyMessage(content, meta);
    for await (const message of stream) {
      await handleClaudeMessage(message, state, baseMeta, notify);
    }
    const finalText = await finalizeClaudeRun(state, baseMeta, prompt, notify);
    if (state.sessionID && resumeKey) {
      sessionByThread.set(resumeKey, state.sessionID);
    }
    if (state.isError) {
      throw new Error(`Claude Code reported an error: ${firstNonEmpty(state.errorText, finalText, "unknown error")}`);
    }
    return {
      agentID: req.params.agentID,
      _meta: {
        ...baseMeta,
        claudeCode: {
          sessionID: state.sessionID || undefined,
          model: state.model || undefined,
          cwd: state.cwd || undefined,
        },
      },
      content: textContent(finalText.trim() || "Claude Code completed without text output."),
    };
  } finally {
    if (timeout) {
      clearTimeout(timeout);
    }
  }
}

function cwdFromMeta(meta: Meta): string | undefined {
  for (const key of ["cwd", "CWD", "workspace", "workspaceRoot"]) {
    const value = metaString(meta, key);
    if (value && value !== "<nil>") {
      return value;
    }
  }
  return undefined;
}

function sessionKey(meta: Meta): string {
  return firstNonEmpty(
    metaString(meta, "threadID"),
    metaString(meta, "chatPath"),
    metaString(meta, "path"),
  );
}

function firstNonEmpty(...values: string[]): string {
  for (const value of values) {
    if (value.trim()) {
      return value;
    }
  }
  return "";
}
