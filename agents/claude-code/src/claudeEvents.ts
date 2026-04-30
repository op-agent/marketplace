import {
  jsonContent,
  mergeMeta,
  metaString,
  textContent,
  userMessage,
  type Content,
  type Meta,
  type Message,
  type TurnResultPayload,
  type TurnResultToolResult,
} from "@op-agent/opagent-protocol";

export interface ClaudeRunState {
  assistantText: string;
  reasoningText: string;
  finalText: string;
  errorText: string;
  sessionID: string;
  model: string;
  cwd: string;
  isError: boolean;
  textStarted: boolean;
  textEnded: boolean;
  textDeltaSeen: boolean;
  turnResultSent: boolean;
  stepSeq: number;
  blockTypes: Map<number, string>;
  blockText: Map<number, string>;
  blockToolID: Map<number, string>;
  blockToolArgs: Map<number, string>;
  toolNamesByID: Map<string, string>;
  toolArgsByID: Map<string, Record<string, unknown>>;
  completedToolIDs: Set<string>;
  toolResults: TurnResultToolResult[];
}

export interface NotifyEvent {
  content: Content;
  meta: Meta;
}

export type Notify = (content: Content, meta: Meta) => Promise<void> | void;

export function createClaudeRunState(): ClaudeRunState {
  return {
    assistantText: "",
    reasoningText: "",
    finalText: "",
    errorText: "",
    sessionID: "",
    model: "",
    cwd: "",
    isError: false,
    textStarted: false,
    textEnded: false,
    textDeltaSeen: false,
    turnResultSent: false,
    stepSeq: 0,
    blockTypes: new Map(),
    blockText: new Map(),
    blockToolID: new Map(),
    blockToolArgs: new Map(),
    toolNamesByID: new Map(),
    toolArgsByID: new Map(),
    completedToolIDs: new Set(),
    toolResults: [],
  };
}

export async function handleClaudeMessage(
  message: unknown,
  state: ClaudeRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  const msg = asRecord(message);
  const type = stringField(msg, "type");
  const sessionID = stringField(msg, "session_id");
  if (sessionID) {
    state.sessionID = sessionID;
  }

  switch (type) {
    case "stream_event":
      await handleStreamEvent(asRecord(msg.event), state, baseMeta, notify);
      break;
    case "assistant":
      await handleAssistantMessage(asRecord(msg.message), state, baseMeta, notify);
      break;
    case "user":
      await handleUserMessage(asRecord(msg.message), state, baseMeta, notify);
      break;
    case "system":
      await handleSystemMessage(msg, state, baseMeta, notify);
      break;
    case "result":
      await handleResultMessage(msg, state, baseMeta, notify);
      break;
    case "tool_progress":
    case "tool_use_summary":
    case "auth_status":
    case "prompt_suggestion":
      await notifyRaw(message, state, baseMeta, notify);
      break;
    default:
      await notifyRaw(message, state, baseMeta, notify);
      break;
  }
}

export async function finalizeClaudeRun(
  state: ClaudeRunState,
  baseMeta: Meta,
  prompt: string,
  notify: Notify,
): Promise<string> {
  const finalText = firstNonEmpty(state.finalText, state.assistantText);
  if (!state.textDeltaSeen && finalText.trim()) {
    await emitTextDelta(finalText, state, baseMeta, notify);
  }
  if (state.textStarted && !state.textEnded) {
    await notify(textContent(finalText), eventMeta(baseMeta, "text_end", { claudeCode: { kind: "assistant" } }));
    state.textEnded = true;
  }
  if (!state.turnResultSent && finalText.trim()) {
    await emitTurnResult(state, baseMeta, prompt, finalText, notify);
  }
  await notify(textContent(""), eventMeta(baseMeta, "done", { claudeCode: { kind: "done" } }));
  return finalText;
}

async function handleStreamEvent(
  event: Record<string, unknown>,
  state: ClaudeRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  const type = stringField(event, "type");
  const index = numberField(event, "index");
  switch (type) {
    case "message_start": {
      const message = asRecord(event.message);
      state.model = stringField(message, "model") || state.model;
      break;
    }
    case "content_block_start": {
      const block = asRecord(event.content_block);
      const blockType = stringField(block, "type");
      state.blockTypes.set(index, blockType);
      if (blockType === "text") {
        await emitTextStart(state, baseMeta, notify, index);
      } else if (blockType === "thinking") {
        await notify(textContent(""), eventMeta(baseMeta, "thinking_start", { contentIndex: index }));
      } else if (blockType === "tool_use") {
        const id = stringField(block, "id") || `tool-${index}`;
        const name = stringField(block, "name") || "tool";
        state.blockToolID.set(index, id);
        state.toolNamesByID.set(id, name);
        const input = asRecord(block.input);
        if (Object.keys(input).length > 0) {
          state.toolArgsByID.set(id, input);
        }
        await emitToolCallStart(id, name, state, baseMeta, notify, index);
      }
      break;
    }
    case "content_block_delta": {
      const delta = asRecord(event.delta);
      const deltaType = stringField(delta, "type");
      if (deltaType === "text_delta") {
        await emitTextDelta(stringField(delta, "text"), state, baseMeta, notify, index);
      } else if (deltaType === "thinking_delta") {
        const text = stringField(delta, "thinking");
        state.reasoningText += text;
        state.blockText.set(index, `${state.blockText.get(index) || ""}${text}`);
        await notify(textContent(text), eventMeta(baseMeta, "thinking_delta", { contentIndex: index }));
      } else if (deltaType === "input_json_delta") {
        const partial = stringField(delta, "partial_json");
        state.blockToolArgs.set(index, `${state.blockToolArgs.get(index) || ""}${partial}`);
        const id = state.blockToolID.get(index) || `tool-${index}`;
        const name = state.toolNamesByID.get(id) || "tool";
        await notify(textContent(partial), eventMeta(baseMeta, "toolcall_delta", { id, name, contentIndex: index }));
      }
      break;
    }
    case "content_block_stop": {
      const blockType = state.blockTypes.get(index);
      if (blockType === "text") {
        await notify(textContent(state.blockText.get(index) || ""), eventMeta(baseMeta, "text_end", { contentIndex: index }));
        state.textEnded = true;
      } else if (blockType === "thinking") {
        await notify(textContent(state.blockText.get(index) || ""), eventMeta(baseMeta, "thinking_end", { contentIndex: index }));
      } else if (blockType === "tool_use") {
        const id = state.blockToolID.get(index) || `tool-${index}`;
        const name = state.toolNamesByID.get(id) || "tool";
        const rawArgs = state.blockToolArgs.get(index) || "";
        const parsedArgs = parseObjectJSON(rawArgs);
        if (parsedArgs) {
          state.toolArgsByID.set(id, parsedArgs);
        }
        await notify(textContent(rawArgs), eventMeta(baseMeta, "toolcall_end", { id, name, contentIndex: index }));
        state.completedToolIDs.add(id);
      }
      break;
    }
  }
}

async function handleAssistantMessage(
  message: Record<string, unknown>,
  state: ClaudeRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  state.model = stringField(message, "model") || state.model;
  const blocks = arrayField(message, "content");
  for (let index = 0; index < blocks.length; index += 1) {
    const block = asRecord(blocks[index]);
    const type = stringField(block, "type");
    if (type === "text") {
      const text = stringField(block, "text");
      if (!state.textDeltaSeen && text) {
        await emitTextDelta(text, state, baseMeta, notify, index);
      }
    } else if (type === "thinking") {
      const text = stringField(block, "thinking") || stringField(block, "text");
      if (text && !state.reasoningText.includes(text)) {
        await notify(textContent(""), eventMeta(baseMeta, "thinking_start", { contentIndex: index }));
        await notify(textContent(text), eventMeta(baseMeta, "thinking_delta", { contentIndex: index }));
        await notify(textContent(text), eventMeta(baseMeta, "thinking_end", { contentIndex: index }));
        state.reasoningText += text;
      }
    } else if (type === "tool_use") {
      const id = stringField(block, "id") || `tool-${index}`;
      if (state.completedToolIDs.has(id)) {
        continue;
      }
      const name = stringField(block, "name") || "tool";
      const input = asRecord(block.input);
      state.toolNamesByID.set(id, name);
      state.toolArgsByID.set(id, input);
      await emitToolCallStart(id, name, state, baseMeta, notify, index);
      const rawArgs = JSON.stringify(input);
      await notify(textContent(rawArgs), eventMeta(baseMeta, "toolcall_delta", { id, name, contentIndex: index }));
      await notify(textContent(rawArgs), eventMeta(baseMeta, "toolcall_end", { id, name, contentIndex: index }));
      state.completedToolIDs.add(id);
    }
  }
}

async function handleUserMessage(
  message: Record<string, unknown>,
  state: ClaudeRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  const blocks = Array.isArray(message.content) ? message.content : [];
  for (const blockValue of blocks) {
    const block = asRecord(blockValue);
    if (stringField(block, "type") !== "tool_result") {
      continue;
    }
    const toolCallID = stringField(block, "tool_use_id");
    const name = state.toolNamesByID.get(toolCallID) || "tool";
    const resultText = toolResultText(block.content);
    const isError = block.is_error === true;
    const result: TurnResultToolResult = {
      toolName: name,
      argumentsObject: state.toolArgsByID.get(toolCallID),
      resultText,
      isError: isError || undefined,
    };
    state.toolResults.push(result);
    const msg: Message = {
      role: "tool",
      tool_call_id: toolCallID,
      name,
      content: resultText,
    };
    state.stepSeq += 1;
    await notify(jsonContent(msg), eventMeta(baseMeta, "tool_result_step", {
      name,
      stepSeq: state.stepSeq,
      argumentsObject: result.argumentsObject,
    }));
  }
}

async function handleSystemMessage(
  msg: Record<string, unknown>,
  state: ClaudeRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  const subtype = stringField(msg, "subtype");
  if (subtype === "init") {
    state.sessionID = stringField(msg, "session_id") || state.sessionID;
    state.model = stringField(msg, "model") || state.model;
    state.cwd = stringField(msg, "cwd") || state.cwd;
  }
  await notifyRaw(msg, state, baseMeta, notify);
}

async function handleResultMessage(
  msg: Record<string, unknown>,
  state: ClaudeRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  state.finalText = stringField(msg, "result") || state.finalText;
  state.isError = msg.is_error === true || stringField(msg, "subtype").startsWith("error");
  if (state.isError) {
    state.errorText = firstNonEmpty(
      stringField(msg, "error"),
      arrayField(msg, "errors").map(String).join("\n"),
      state.finalText,
    );
  }
  const usage = asRecord(msg.usage);
  const inputTokens = numberField(usage, "input_tokens");
  const outputTokens = numberField(usage, "output_tokens");
  const totalTokens = numberField(usage, "total_tokens") || inputTokens + outputTokens;
  if (totalTokens > 0) {
    await notify(textContent(""), eventMeta(baseMeta, "tokenUsage", {
      loopInputTokens: String(inputTokens),
      loopOutputTokens: String(outputTokens),
      loopTotalTokens: String(totalTokens),
    }));
  }
}

async function emitTextStart(
  state: ClaudeRunState,
  baseMeta: Meta,
  notify: Notify,
  contentIndex?: number,
): Promise<void> {
  if (!state.textStarted || state.textEnded) {
    await notify(textContent(""), eventMeta(baseMeta, "text_start", optionalIndex(contentIndex)));
    state.textStarted = true;
    state.textEnded = false;
  }
}

async function emitTextDelta(
  text: string,
  state: ClaudeRunState,
  baseMeta: Meta,
  notify: Notify,
  contentIndex?: number,
): Promise<void> {
  if (!text) {
    return;
  }
  await emitTextStart(state, baseMeta, notify, contentIndex);
  state.assistantText += text;
  state.textDeltaSeen = true;
  if (contentIndex != null) {
    state.blockText.set(contentIndex, `${state.blockText.get(contentIndex) || ""}${text}`);
  }
  await notify(textContent(text), eventMeta(baseMeta, "text_delta", optionalIndex(contentIndex)));
}

async function emitToolCallStart(
  id: string,
  name: string,
  state: ClaudeRunState,
  baseMeta: Meta,
  notify: Notify,
  contentIndex?: number,
): Promise<void> {
  state.stepSeq += 1;
  await notify(textContent(""), eventMeta(baseMeta, "toolcall_start", {
    id,
    name,
    stepSeq: state.stepSeq,
    ...optionalIndex(contentIndex),
  }));
}

async function emitTurnResult(
  state: ClaudeRunState,
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
    title: firstNonEmpty(metaString(baseMeta, "title"), "Claude Code"),
    parentThreadID: metaString(baseMeta, "parentThreadID") || undefined,
    planTurn: baseMeta.planTurn === true || undefined,
    userMessage: userMessage(prompt),
    assistantText,
    reasoningText: state.reasoningText || undefined,
    toolResults: state.toolResults.length > 0 ? state.toolResults : undefined,
  };
  await notify(jsonContent(payload), eventMeta(baseMeta, "turn_result", {
    turnID: payload.turnID,
    claudeCode: {
      kind: "turn_result",
      sessionID: state.sessionID || undefined,
      model: state.model || undefined,
      cwd: state.cwd || undefined,
    },
  }));
  state.turnResultSent = true;
}

async function notifyRaw(
  message: unknown,
  _state: ClaudeRunState,
  baseMeta: Meta,
  notify: Notify,
): Promise<void> {
  if (baseMeta.claudeCodeNotifyRawEvents !== true) {
    return;
  }
  await notify(jsonContent(message), eventMeta(baseMeta, "ignore", { claudeCode: { kind: "raw" } }));
}

function eventMeta(baseMeta: Meta, type: string, extra: Meta = {}): Meta {
  return mergeMeta(baseMeta, { type, ...extra });
}

function optionalIndex(index: number | undefined): Meta {
  return index == null ? {} : { contentIndex: index };
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {};
}

function arrayField(obj: Record<string, unknown>, key: string): unknown[] {
  const value = obj[key];
  return Array.isArray(value) ? value : [];
}

function stringField(obj: Record<string, unknown>, key: string): string {
  const value = obj[key];
  if (value == null) {
    return "";
  }
  return typeof value === "string" ? value : String(value);
}

function numberField(obj: Record<string, unknown>, key: string): number {
  const value = obj[key];
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number.parseInt(value, 10);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

function firstNonEmpty(...values: string[]): string {
  for (const value of values) {
    if (value.trim()) {
      return value;
    }
  }
  return "";
}

function parseObjectJSON(value: string): Record<string, unknown> | null {
  try {
    const parsed = JSON.parse(value);
    return asRecord(parsed);
  } catch {
    return null;
  }
}

function toolResultText(value: unknown): string {
  if (typeof value === "string") {
    return value;
  }
  if (Array.isArray(value)) {
    return value.map((item) => {
      const block = asRecord(item);
      if (stringField(block, "type") === "text") {
        return stringField(block, "text");
      }
      return JSON.stringify(item);
    }).filter(Boolean).join("\n");
  }
  return value == null ? "" : JSON.stringify(value);
}
