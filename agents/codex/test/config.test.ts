import { describe, expect, it } from "vitest";
import { buildCodexOptions, buildThreadOptions, configFromEnv } from "../src/config.js";

describe("configFromEnv", () => {
  it("keeps Codex defaults unset unless explicitly configured", () => {
    const cfg = configFromEnv({});
    expect(cfg.model).toBeUndefined();
    expect(cfg.sandboxMode).toBeUndefined();
    expect(cfg.appendOpAgentPrompt).toBe(true);
    expect(cfg.useLoginShell).toBe(true);
    expect(cfg.resumeSessions).toBe(true);
  });

  it("parses thread and client options", () => {
    const cfg = configFromEnv({
      CODEX_AGENT_MODEL: "gpt-5.4",
      CODEX_AGENT_REASONING_EFFORT: "high",
      CODEX_AGENT_SANDBOX_MODE: "workspace-write",
      CODEX_AGENT_APPROVAL_POLICY: "on-request",
      CODEX_AGENT_WEB_SEARCH_MODE: "live",
      CODEX_AGENT_NETWORK_ACCESS: "true",
      CODEX_AGENT_SKIP_GIT_REPO_CHECK: "true",
      CODEX_AGENT_ADDITIONAL_DIRECTORIES: "/a,/b",
      CODEX_AGENT_CODEX_PATH: "/opt/codex",
      CODEX_AGENT_BASE_URL: "https://example.test",
      CODEX_AGENT_API_KEY: "secret",
      CODEX_AGENT_TIMEOUT_SECONDS: "12",
    });

    expect(buildThreadOptions(cfg, "/repo")).toMatchObject({
      model: "gpt-5.4",
      modelReasoningEffort: "high",
      sandboxMode: "workspace-write",
      approvalPolicy: "on-request",
      webSearchMode: "live",
      networkAccessEnabled: true,
      skipGitRepoCheck: true,
      additionalDirectories: ["/a", "/b"],
      workingDirectory: "/repo",
    });
    expect(cfg.codexPathOverride).toBe("/opt/codex");
    expect(cfg.baseUrl).toBe("https://example.test");
    expect(cfg.apiKey).toBe("secret");
    expect(cfg.timeoutMs).toBe(12_000);
  });

  it("appends the OpAgent prompt to developer instructions", async () => {
    const cfg = configFromEnv({
      CODEX_AGENT_USE_LOGIN_SHELL: "false",
      CODEX_AGENT_CONFIG_JSON: "{\"developer_instructions\":\"existing\"}",
    });
    const options = await buildCodexOptions(cfg, "agent prompt");
    expect(options.env).toBeUndefined();
    expect(options.config?.developer_instructions).toBe("existing\n\nagent prompt");
  });
});
