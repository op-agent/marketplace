import { describe, expect, it } from "vitest";
import type { ThreadEvent } from "@openai/codex-sdk";
import {
  createCodexRunState,
  finalizeCodexRun,
  handleCodexEvent,
  redactSensitive,
  type NotifyEvent,
} from "../src/codexEvents.js";

describe("codex event bridge", () => {
  it("translates text, reasoning, tool lifecycle, usage, and turn result events", async () => {
    const state = createCodexRunState();
    const events: NotifyEvent[] = [];
    const notify = (content: NotifyEvent["content"], meta: NotifyEvent["meta"]) => {
      events.push({ content, meta });
    };
    const meta = { threadID: "thread-1", agentID: "agent-1", chatPath: "/tmp/chat.md" };

    const stream: ThreadEvent[] = [
      { type: "thread.started", thread_id: "codex-thread-1" },
      { type: "item.started", item: { id: "r1", type: "reasoning", text: "plan" } },
      { type: "item.completed", item: { id: "r1", type: "reasoning", text: "plan" } },
      {
        type: "item.started",
        item: {
          id: "cmd1",
          type: "command_execution",
          command: "npm test",
          aggregated_output: "",
          status: "in_progress",
        },
      },
      {
        type: "item.completed",
        item: {
          id: "cmd1",
          type: "command_execution",
          command: "npm test",
          aggregated_output: "ok",
          exit_code: 0,
          status: "completed",
        },
      },
      { type: "item.started", item: { id: "m1", type: "agent_message", text: "hello" } },
      { type: "item.updated", item: { id: "m1", type: "agent_message", text: "hello world" } },
      { type: "item.completed", item: { id: "m1", type: "agent_message", text: "hello world" } },
      {
        type: "turn.completed",
        usage: {
          input_tokens: 2,
          cached_input_tokens: 1,
          output_tokens: 3,
          reasoning_output_tokens: 4,
        },
      },
    ];
    for (const event of stream) {
      await handleCodexEvent(event, state, meta, notify);
    }
    await finalizeCodexRun(state, meta, "prompt", notify);

    expect(state.threadID).toBe("codex-thread-1");
    expect(state.assistantText).toBe("hello world");
    expect(events.map((event) => event.meta.type)).toEqual([
      "thinking_start",
      "thinking_delta",
      "thinking_end",
      "toolcall_start",
      "toolcall_end",
      "tool_result_step",
      "text_start",
      "text_delta",
      "text_delta",
      "text_end",
      "tokenUsage",
      "turn_result",
      "done",
    ]);
    const turnResult = events.find((event) => event.meta.type === "turn_result");
    expect(turnResult?.content.type).toBe("json");
    expect(state.toolResults).toHaveLength(1);
  });

  it("redacts common secret shapes", () => {
    expect(redactSensitive("OPENAI_API_KEY=sk-testsecret123 Bearer abc.def")).toBe("OPENAI_API_KEY=<redacted> Bearer <redacted>");
  });
});
