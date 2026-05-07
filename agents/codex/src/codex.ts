import { Codex, type Input, type Thread } from "@openai/codex-sdk";
import {
  cloneMeta,
  contentText,
  metaString,
  textContent,
  type CallAgentParams,
  type CallAgentResult,
  type Meta,
  type ServerRequest,
} from "@op-agent/opagent-protocol";
import { buildCodexOptions, buildThreadOptions, type CodexAgentConfig } from "./config.js";
import {
  createCodexRunState,
  finalizeCodexRun,
  handleCodexEvent,
  redactSensitive,
} from "./codexEvents.js";

const threadIDByOpAgentThread = new Map<string, string>();

export async function runCodexAgent(
  req: ServerRequest<CallAgentParams>,
  cfg: CodexAgentConfig,
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
    baseMeta.codexNotifyRawEvents = true;
  }

  const abortController = new AbortController();
  const timeout = cfg.timeoutMs
    ? setTimeout(() => abortController.abort(), cfg.timeoutMs)
    : undefined;

  try {
    const codexOptions = await buildCodexOptions(cfg, agentPrompt);
    const codex = new Codex(codexOptions);
    const cwd = cwdFromMeta(baseMeta);
    const threadOptions = buildThreadOptions(cfg, cwd);
    const resumeKey = sessionKey(baseMeta);
    const explicitThreadID = firstNonEmpty(
      metaString(baseMeta, "codexThreadID"),
      nestedMetaString(baseMeta, "codex", "threadID"),
    );
    const resumeThreadID = cfg.resumeSessions
      ? firstNonEmpty(explicitThreadID, threadIDByOpAgentThread.get(resumeKey) || "")
      : "";
    const thread = resumeThreadID
      ? codex.resumeThread(resumeThreadID, threadOptions)
      : codex.startThread(threadOptions);

    const state = createCodexRunState();
    const notify = (content: Parameters<typeof req.session.notifyMessage>[0], meta: Meta) => req.session.notifyMessage(content, meta);
    const { events } = await thread.runStreamed(buildInput(prompt), { signal: abortController.signal });
    for await (const event of events) {
      await handleCodexEvent(event, state, baseMeta, notify);
    }
    if (!state.threadID) {
      state.threadID = threadID(thread);
    }
    if (state.threadID && resumeKey) {
      threadIDByOpAgentThread.set(resumeKey, state.threadID);
    }
    if (state.isError) {
      throw new Error(`Codex reported an error: ${firstNonEmpty(state.errorText, "unknown error")}`);
    }

    const finalText = await finalizeCodexRun(state, baseMeta, prompt, notify);
    return {
      agentID: req.params.agentID,
      _meta: {
        ...baseMeta,
        codex: {
          threadID: state.threadID || undefined,
          model: cfg.model || undefined,
          cwd: cwd || undefined,
        },
      },
      content: textContent(finalText.trim() || "Codex completed without text output."),
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(redactSensitive(message));
  } finally {
    if (timeout) {
      clearTimeout(timeout);
    }
  }
}

function buildInput(prompt: string): Input {
  return prompt;
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

function nestedMetaString(meta: Meta, outer: string, inner: string): string {
  const value = meta[outer];
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return "";
  }
  const nested = value as Record<string, unknown>;
  const text = nested[inner];
  return text == null ? "" : String(text).trim();
}

function threadID(thread: Thread): string {
  return thread.id || "";
}

function firstNonEmpty(...values: string[]): string {
  for (const value of values) {
    if (value.trim()) {
      return value;
    }
  }
  return "";
}
