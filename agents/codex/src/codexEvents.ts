import {
  jsonContent,
  mergeMeta,
  metaString,
  textContent,
  userMessage,
  type Content,
  type Message,
  type Meta,
  type TurnResultPayload,
  type TurnResultToolResult,
} from "@op-agent/opagent-protocol";
import type { ThreadEvent, ThreadItem, Usage } from "@openai/codex-sdk";

export interface CodexRunState {
  assistantText: string;
  reasoningText: string;
  finalText: string;
  errorText: string;
  threadID: string;
  isError: boolean;
  textStarted: boolean;
  textEnded: boolean;
  thinkingStarted: boolean;
  thinkingEnded: boolean;
  turnResultSent: boolean;
  stepSeq: number;
  textByItemID: Map<string, string>;
  reasoningByItemID: Map<string, string>;
  startedToolIDs: Set<string>;
  completedToolIDs: Set<string>;
  toolResults: TurnResultToolResult[];
}

export interface NotifyEvent {
  content: Content;
  meta: Meta;
}

export type Notify = (content: Content, meta: Meta) => Promise<void> | void;

export function createCodexRunState(): CodexRunState {
  return {
    assistantText: "",
    reasoningText: "",
    finalText: "",
    errorText: "",
    threadID: "",
    isError: false,
    textStarted: false,
    textEnded: false,
    thinkingStarted: false,
    thinkingEnded: false,
    turnResultSent: false,
    stepSeq: 0,
    textByItemID: new Map(),
    reasoningByItemID: new Map(),
    startedToolIDs: new Set(),
    completedToolIDs: new Set(),
    toolResults: [],
  };
}

export async function handleCodexEvent(
  event: ThreadEvent,
  state: CodexRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  switch (event.type) {
    case "thread.started":
      state.threadID = event.thread_id;
      await notifyRaw(event, baseMeta, notify);
      break;
    case "turn.started":
      await notifyRaw(event, baseMeta, notify);
      break;
    case "item.started":
    case "item.updated":
    case "item.completed":
      await handleItemEvent(event.type, event.item, state, baseMeta, notify);
      break;
    case "turn.completed":
      await emitUsage(event.usage, baseMeta, notify);
      break;
    case "turn.failed":
      state.isError = true;
      state.errorText = redactSensitive(event.error.message);
      break;
    case "error":
      state.isError = true;
      state.errorText = redactSensitive(event.message);
      break;
  }
}

export async function finalizeCodexRun(
  state: CodexRunState,
  baseMeta: Meta,
  prompt: string,
  notify: Notify,
): Promise<string> {
  const finalText = firstNonEmpty(state.finalText, state.assistantText, "Codex completed without text output.");
  if (state.textStarted && !state.textEnded) {
    await notify(textContent(finalText), eventMeta(baseMeta, "text_end", { codex: { kind: "assistant" } }));
    state.textEnded = true;
  }
  if (state.thinkingStarted && !state.thinkingEnded) {
    await notify(textContent(state.reasoningText), eventMeta(baseMeta, "thinking_end", { codex: { kind: "reasoning" } }));
    state.thinkingEnded = true;
  }
  if (!state.turnResultSent) {
    await emitTurnResult(state, baseMeta, prompt, finalText, notify);
  }
  await notify(textContent(""), eventMeta(baseMeta, "done", { codex: { kind: "done" } }));
  return finalText;
}

async function handleItemEvent(
  eventType: "item.started" | "item.updated" | "item.completed",
  item: ThreadItem,
  state: CodexRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  switch (item.type) {
    case "agent_message":
      await handleAgentMessageItem(eventType, item.id, item.text, state, baseMeta, notify);
      break;
    case "reasoning":
      await handleReasoningItem(eventType, item.id, item.text, state, baseMeta, notify);
      break;
    case "command_execution":
      await handleCommandItem(eventType, item, state, baseMeta, notify);
      break;
    case "file_change":
      await handleFileChangeItem(eventType, item, state, baseMeta, notify);
      break;
    case "mcp_tool_call":
      await handleMcpToolItem(eventType, item, state, baseMeta, notify);
      break;
    case "web_search":
      await handleWebSearchItem(eventType, item, state, baseMeta, notify);
      break;
    case "todo_list":
      await notifyRaw({ type: eventType, item }, baseMeta, notify);
      break;
    case "error":
      state.errorText = redactSensitive(firstNonEmpty(state.errorText, item.message));
      await notifyRaw({ type: eventType, item }, baseMeta, notify);
      break;
  }
}

async function handleAgentMessageItem(
  eventType: string,
  id: string,
  text: string,
  state: CodexRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  const previous = state.textByItemID.get(id) || "";
  const delta = text.startsWith(previous) ? text.slice(previous.length) : text;
  if (delta) {
    await emitTextDelta(delta, state, baseMeta, notify);
    state.textByItemID.set(id, text);
  }
  if (eventType === "item.completed") {
    state.finalText = text || state.finalText;
    if (state.textStarted && !state.textEnded) {
      await notify(textContent(text), eventMeta(baseMeta, "text_end", { codex: { kind: "assistant" } }));
      state.textEnded = true;
    }
  }
}

async function handleReasoningItem(
  eventType: string,
  id: string,
  text: string,
  state: CodexRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  const previous = state.reasoningByItemID.get(id) || "";
  const delta = text.startsWith(previous) ? text.slice(previous.length) : text;
  if (delta) {
    if (!state.thinkingStarted || state.thinkingEnded) {
      await notify(textContent(""), eventMeta(baseMeta, "thinking_start", { codex: { kind: "reasoning" } }));
      state.thinkingStarted = true;
      state.thinkingEnded = false;
    }
    state.reasoningText += delta;
    state.reasoningByItemID.set(id, text);
    await notify(textContent(delta), eventMeta(baseMeta, "thinking_delta", { codex: { kind: "reasoning" } }));
  }
  if (eventType === "item.completed" && state.thinkingStarted && !state.thinkingEnded) {
    await notify(textContent(text), eventMeta(baseMeta, "thinking_end", { codex: { kind: "reasoning" } }));
    state.thinkingEnded = true;
  }
}

async function handleCommandItem(
  eventType: string,
  item: Extract<ThreadItem, { type: "command_execution" }>,
  state: CodexRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  const args = { command: item.command };
  await ensureToolStarted(item.id, "shell", args, state, baseMeta, notify);
  if (eventType === "item.completed" && !state.completedToolIDs.has(item.id)) {
    const resultText = firstNonEmpty(item.aggregated_output, exitText(item.exit_code, item.status));
    await completeTool(item.id, "shell", args, resultText, item.status === "failed", state, baseMeta, notify);
  }
}

async function handleFileChangeItem(
  eventType: string,
  item: Extract<ThreadItem, { type: "file_change" }>,
  state: CodexRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  const args = { changes: item.changes };
  await ensureToolStarted(item.id, "file_change", args, state, baseMeta, notify);
  if (eventType === "item.completed" && !state.completedToolIDs.has(item.id)) {
    const resultText = item.changes.map((change) => `${change.kind}: ${change.path}`).join("\n");
    await completeTool(item.id, "file_change", args, resultText, item.status === "failed", state, baseMeta, notify);
  }
}

async function handleMcpToolItem(
  eventType: string,
  item: Extract<ThreadItem, { type: "mcp_tool_call" }>,
  state: CodexRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  const args = asRecord(item.arguments);
  const name = item.tool || "mcp_tool";
  await ensureToolStarted(item.id, name, { server: item.server, ...args }, state, baseMeta, notify);
  if (eventType === "item.completed" && !state.completedToolIDs.has(item.id)) {
    const resultText = item.error?.message || mcpResultText(item.result);
    await completeTool(item.id, name, args, resultText, item.status === "failed", state, baseMeta, notify);
  }
}

async function handleWebSearchItem(
  eventType: string,
  item: Extract<ThreadItem, { type: "web_search" }>,
  state: CodexRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  const args = { query: item.query };
  await ensureToolStarted(item.id, "web_search", args, state, baseMeta, notify);
  if (eventType === "item.completed" && !state.completedToolIDs.has(item.id)) {
    await completeTool(item.id, "web_search", args, `query: ${item.query}`, false, state, baseMeta, notify);
  }
}

async function emitTextDelta(
  text: string,
  state: CodexRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  if (!state.textStarted || state.textEnded) {
    await notify(textContent(""), eventMeta(baseMeta, "text_start", { codex: { kind: "assistant" } }));
    state.textStarted = true;
    state.textEnded = false;
  }
  state.assistantText += text;
  await notify(textContent(text), eventMeta(baseMeta, "text_delta", { codex: { kind: "assistant" } }));
}

async function ensureToolStarted(
  id: string,
  name: string,
  argumentsObject: Record<string, unknown>,
  state: CodexRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  if (state.startedToolIDs.has(id)) {
    return;
  }
  state.startedToolIDs.add(id);
  state.stepSeq += 1;
  await notify(textContent(""), eventMeta(baseMeta, "toolcall_start", {
    id,
    name,
    stepSeq: state.stepSeq,
    argumentsObject,
    codex: { kind: "tool", itemID: id },
  }));
}

async function completeTool(
  id: string,
  name: string,
  argumentsObject: Record<string, unknown>,
  resultText: string,
  isError: boolean,
  state: CodexRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  state.completedToolIDs.add(id);
  await notify(textContent(resultText), eventMeta(baseMeta, "toolcall_end", {
    id,
    name,
    isError: isError || undefined,
    codex: { kind: "tool", itemID: id },
  }));

  const result: TurnResultToolResult = {
    toolName: name,
    argumentsObject,
    resultText,
    isError: isError || undefined,
  };
  state.toolResults.push(result);
  const msg: Message = {
    role: "tool",
    tool_call_id: id,
    name,
    content: resultText,
  };
  await notify(jsonContent(msg), eventMeta(baseMeta, "tool_result_step", {
    id,
    name,
    stepSeq: state.stepSeq,
    argumentsObject,
    isError: isError || undefined,
    codex: { kind: "tool_result", itemID: id },
  }));
}

async function emitUsage(usage: Usage, baseMeta: Meta, notify: Notify): Promise<void> {
  const inputTokens = usage.input_tokens || 0;
  const outputTokens = usage.output_tokens || 0;
  const totalTokens = inputTokens + outputTokens;
  await notify(textContent(""), eventMeta(baseMeta, "tokenUsage", {
    loopInputTokens: String(inputTokens),
    loopOutputTokens: String(outputTokens),
    loopTotalTokens: String(totalTokens),
    contextTokens: String(totalTokens),
    contextKnown: "true",
    cachedInputTokens: String(usage.cached_input_tokens || 0),
    reasoningOutputTokens: String(usage.reasoning_output_tokens || 0),
  }));
}

async function emitTurnResult(
  state: CodexRunState,
  baseMeta: Meta,
  prompt: string,
  assistantText: string,
  notify: Notify,
): Promise<void> {
  const path = firstNonEmpty(metaString(baseMeta, "path"), metaString(baseMeta, "chatPath"));
  const payload: TurnResultPayload = {
    threadID: metaString(baseMeta, "threadID"),
    fileID: metaString(baseMeta, "fileID") || undefined,
    turnID: firstNonEmpty(metaString(baseMeta, "turnID"), metaString(baseMeta, "turnRequestID")),
    agentID: metaString(baseMeta, "agentID"),
    path: path || undefined,
    chatPath: path || undefined,
    title: firstNonEmpty(metaString(baseMeta, "title"), "Codex"),
    parentThreadID: metaString(baseMeta, "parentThreadID") || undefined,
    planTurn: baseMeta.planTurn === true || undefined,
    userMessage: userMessage(prompt),
    assistantText,
    reasoningText: state.reasoningText || undefined,
    toolResults: state.toolResults.length > 0 ? state.toolResults : undefined,
  };
  await notify(jsonContent(payload), eventMeta(baseMeta, "turn_result", {
    turnID: payload.turnID,
    codex: {
      kind: "turn_result",
      threadID: state.threadID || undefined,
    },
  }));
  state.turnResultSent = true;
}

async function notifyRaw(message: unknown, baseMeta: Meta, notify: Notify): Promise<void> {
  if (baseMeta.codexNotifyRawEvents !== true) {
    return;
  }
  await notify(jsonContent(message), eventMeta(baseMeta, "ignore", { codex: { kind: "raw" } }));
}

function eventMeta(baseMeta: Meta, type: string, extra: Meta = {}): Meta {
  return mergeMeta(baseMeta, { type, ...extra });
}

function firstNonEmpty(...values: Array<string | undefined>): string {
  for (const value of values) {
    if (value?.trim()) {
      return value;
    }
  }
  return "";
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {};
}

function exitText(exitCode: number | undefined, status: string): string {
  if (exitCode === undefined) {
    return `status: ${status}`;
  }
  return `exit code: ${exitCode}`;
}

function mcpResultText(result: Extract<ThreadItem, { type: "mcp_tool_call" }>["result"]): string {
  if (!result) {
    return "";
  }
  const content = Array.isArray(result.content) ? result.content : [];
  const parts = content.map((block) => {
    if (block && typeof block === "object" && "type" in block && block.type === "text" && "text" in block) {
      return String(block.text);
    }
    return JSON.stringify(block);
  }).filter(Boolean);
  if (parts.length > 0) {
    return parts.join("\n");
  }
  return result.structured_content == null ? "" : JSON.stringify(result.structured_content);
}

export function redactSensitive(text: string): string {
  return text
    .replace(/\b(sk-[A-Za-z0-9_-]{8,})\b/g, "<redacted>")
    .replace(/\b((?:OPENAI|CODEX)_[A-Z0-9_]*KEY=)[^\s]+/gi, "$1<redacted>")
    .replace(/\b(Bearer\s+)[A-Za-z0-9._-]+/gi, "$1<redacted>");
}
