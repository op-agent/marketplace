import { describe, expect, it } from "vitest";
import { configFromEnv } from "../src/config.js";

describe("configFromEnv", () => {
  it("defaults to the user-installed claude executable", () => {
    expect(configFromEnv({}).pathToClaudeCodeExecutable).toBe("claude");
  });

  it("keeps explicit executable and maps yolo permissions", () => {
    const cfg = configFromEnv({
      CLAUDE_CODE_CLI: "/opt/claude/bin/claude",
      CLAUDE_CODE_PERMISSION_MODE: "yolo",
    });
    expect(cfg.pathToClaudeCodeExecutable).toBe("/opt/claude/bin/claude");
    expect(cfg.permissionMode).toBe("bypassPermissions");
  });
});
