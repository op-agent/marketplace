import { describe, expect, it } from "vitest";
import {
  createClaudeRunState,
  finalizeClaudeRun,
  handleClaudeMessage,
  type NotifyEvent,
} from "../src/claudeEvents.js";

describe("claude event bridge", () => {
  it("translates partial text and tool lifecycle events", async () => {
    const state = createClaudeRunState();
    const events: NotifyEvent[] = [];
    const notify = (content: NotifyEvent["content"], meta: NotifyEvent["meta"]) => {
      events.push({ content, meta });
    };
    const meta = { threadID: "thread-1", agentID: "agent-1", chatPath: "/tmp/chat.md" };

    await handleClaudeMessage({
      type: "stream_event",
      session_id: "sess-1",
      event: { type: "content_block_start", index: 0, content_block: { type: "text" } },
    }, state, meta, notify);
    await handleClaudeMessage({
      type: "stream_event",
      session_id: "sess-1",
      event: { type: "content_block_delta", index: 0, delta: { type: "text_delta", text: "hello" } },
    }, state, meta, notify);
    await handleClaudeMessage({
      type: "stream_event",
      session_id: "sess-1",
      event: { type: "content_block_start", index: 1, content_block: { type: "tool_use", id: "toolu_1", name: "Read" } },
    }, state, meta, notify);
    await handleClaudeMessage({
      type: "stream_event",
      session_id: "sess-1",
      event: { type: "content_block_delta", index: 1, delta: { type: "input_json_delta", partial_json: "{\"file_path\":\"a.ts\"}" } },
    }, state, meta, notify);
    await handleClaudeMessage({
      type: "stream_event",
      session_id: "sess-1",
      event: { type: "content_block_stop", index: 1 },
    }, state, meta, notify);
    await handleClaudeMessage({
      type: "result",
      subtype: "success",
      is_error: false,
      result: "hello",
      usage: { input_tokens: 2, output_tokens: 3, total_tokens: 5 },
    }, state, meta, notify);
    await finalizeClaudeRun(state, meta, "prompt", notify);

    expect(events.map((event) => event.meta.type)).toEqual([
      "text_start",
      "text_delta",
      "toolcall_start",
      "toolcall_delta",
      "toolcall_end",
      "tokenUsage",
      "text_end",
      "turn_result",
      "done",
    ]);
    expect(state.sessionID).toBe("sess-1");
  });
});
