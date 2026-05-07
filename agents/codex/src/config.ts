import { execFile } from "node:child_process";
import os from "node:os";
import { promisify } from "node:util";
import type {
  ApprovalMode,
  CodexOptions,
  ModelReasoningEffort,
  SandboxMode,
  ThreadOptions,
  WebSearchMode,
} from "@openai/codex-sdk";

const execFileAsync = promisify(execFile);

type CodexConfig = NonNullable<CodexOptions["config"]>;
type CodexConfigValue = CodexConfig[string];

export interface CodexAgentConfig {
  model?: string;
  modelReasoningEffort?: ModelReasoningEffort;
  sandboxMode?: SandboxMode;
  approvalPolicy?: ApprovalMode;
  webSearchMode?: WebSearchMode;
  networkAccessEnabled?: boolean;
  skipGitRepoCheck?: boolean;
  additionalDirectories?: string[];
  codexPathOverride?: string;
  baseUrl?: string;
  apiKey?: string;
  config?: CodexConfig;
  appendOpAgentPrompt: boolean;
  notifyRawEvents: boolean;
  useLoginShell: boolean;
  shell: string;
  shellFlags: string;
  timeoutMs?: number;
  resumeSessions: boolean;
}

export function configFromEnv(env: NodeJS.ProcessEnv = process.env): CodexAgentConfig {
  return {
    model: firstEnv(env, "CODEX_AGENT_MODEL") || undefined,
    modelReasoningEffort: enumEnv<ModelReasoningEffort>(env, "CODEX_AGENT_REASONING_EFFORT", [
      "minimal",
      "low",
      "medium",
      "high",
      "xhigh",
    ]),
    sandboxMode: enumEnv<SandboxMode>(env, "CODEX_AGENT_SANDBOX_MODE", [
      "read-only",
      "workspace-write",
      "danger-full-access",
    ]),
    approvalPolicy: enumEnv<ApprovalMode>(env, "CODEX_AGENT_APPROVAL_POLICY", [
      "never",
      "on-request",
      "on-failure",
      "untrusted",
    ]),
    webSearchMode: enumEnv<WebSearchMode>(env, "CODEX_AGENT_WEB_SEARCH_MODE", [
      "disabled",
      "cached",
      "live",
    ]),
    networkAccessEnabled: boolEnvOptional(env, "CODEX_AGENT_NETWORK_ACCESS"),
    skipGitRepoCheck: boolEnvOptional(env, "CODEX_AGENT_SKIP_GIT_REPO_CHECK"),
    additionalDirectories: csvEnv(env, "CODEX_AGENT_ADDITIONAL_DIRECTORIES"),
    codexPathOverride: firstEnv(env, "CODEX_AGENT_CODEX_PATH", "CODEX_AGENT_CODEX_CLI") || undefined,
    baseUrl: firstEnv(env, "CODEX_AGENT_BASE_URL") || undefined,
    apiKey: firstEnv(env, "CODEX_AGENT_API_KEY") || undefined,
    config: configJSONEnv(env, "CODEX_AGENT_CONFIG_JSON"),
    appendOpAgentPrompt: boolEnv(env, "CODEX_AGENT_APPEND_OPAGENT_PROMPT", true),
    notifyRawEvents: boolEnv(env, "CODEX_AGENT_NOTIFY_RAW_EVENTS", false),
    useLoginShell: boolEnv(env, "CODEX_AGENT_USE_LOGIN_SHELL", true),
    shell: firstEnv(env, "CODEX_AGENT_SHELL", "SHELL") || defaultShell(),
    shellFlags: firstEnv(env, "CODEX_AGENT_SHELL_FLAGS") || "-lic",
    timeoutMs: positiveInt(firstEnv(env, "CODEX_AGENT_TIMEOUT_SECONDS")) != null
      ? positiveInt(firstEnv(env, "CODEX_AGENT_TIMEOUT_SECONDS"))! * 1000
      : undefined,
    resumeSessions: boolEnv(env, "CODEX_AGENT_RESUME_SESSIONS", true),
  };
}

export async function buildCodexOptions(
  cfg: CodexAgentConfig,
  agentPrompt: string,
): Promise<CodexOptions> {
  const config = cloneConfig(cfg.config);
  if (cfg.appendOpAgentPrompt && agentPrompt.trim()) {
    appendDeveloperInstructions(config, agentPrompt);
  }

  const env = cfg.useLoginShell
    ? await captureLoginShellEnv(cfg.shell, cfg.shellFlags).catch((error: unknown) => {
      console.error(`codex agent: login shell env capture failed: ${error instanceof Error ? error.message : String(error)}`);
      return envObject(process.env);
    })
    : undefined;

  return {
    codexPathOverride: cfg.codexPathOverride,
    baseUrl: cfg.baseUrl,
    apiKey: cfg.apiKey,
    config: Object.keys(config).length > 0 ? config : undefined,
    env,
  };
}

export function buildThreadOptions(cfg: CodexAgentConfig, cwd?: string): ThreadOptions {
  return {
    model: cfg.model,
    sandboxMode: cfg.sandboxMode,
    workingDirectory: cwd || undefined,
    skipGitRepoCheck: cfg.skipGitRepoCheck,
    modelReasoningEffort: cfg.modelReasoningEffort,
    networkAccessEnabled: cfg.networkAccessEnabled,
    webSearchMode: cfg.webSearchMode,
    approvalPolicy: cfg.approvalPolicy,
    additionalDirectories: cfg.additionalDirectories,
  };
}

function appendDeveloperInstructions(config: CodexConfig, agentPrompt: string): void {
  const existing = config.developer_instructions;
  if (existing == null) {
    config.developer_instructions = agentPrompt;
    return;
  }
  if (typeof existing !== "string") {
    throw new Error("CODEX_AGENT_CONFIG_JSON developer_instructions must be a string when CODEX_AGENT_APPEND_OPAGENT_PROMPT is enabled");
  }
  config.developer_instructions = `${existing.trim()}\n\n${agentPrompt.trim()}`.trim();
}

function cloneConfig(config: CodexConfig | undefined): CodexConfig {
  if (!config) {
    return {};
  }
  return JSON.parse(JSON.stringify(config)) as CodexConfig;
}

function firstEnv(env: NodeJS.ProcessEnv, ...keys: string[]): string {
  for (const key of keys) {
    const value = env[key]?.trim();
    if (value) {
      return value;
    }
  }
  return "";
}

function boolEnv(env: NodeJS.ProcessEnv, key: string, fallback: boolean): boolean {
  return boolEnvOptional(env, key) ?? fallback;
}

function boolEnvOptional(env: NodeJS.ProcessEnv, key: string): boolean | undefined {
  const value = env[key]?.trim().toLowerCase();
  if (!value) {
    return undefined;
  }
  if (["1", "true", "yes", "y", "on"].includes(value)) {
    return true;
  }
  if (["0", "false", "no", "n", "off"].includes(value)) {
    return false;
  }
  throw new Error(`${key} must be a boolean`);
}

function enumEnv<T extends string>(
  env: NodeJS.ProcessEnv,
  key: string,
  values: readonly T[],
): T | undefined {
  const value = env[key]?.trim();
  if (!value) {
    return undefined;
  }
  if ((values as readonly string[]).includes(value)) {
    return value as T;
  }
  throw new Error(`${key} must be one of: ${values.join(", ")}`);
}

function csvEnv(env: NodeJS.ProcessEnv, key: string): string[] | undefined {
  const value = env[key]?.trim();
  if (!value) {
    return undefined;
  }
  const items = value.split(",").map((item) => item.trim()).filter(Boolean);
  return items.length > 0 ? items : undefined;
}

function positiveInt(value: string): number | undefined {
  if (!value.trim()) {
    return undefined;
  }
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}

function configJSONEnv(env: NodeJS.ProcessEnv, key: string): CodexConfig | undefined {
  const value = env[key]?.trim();
  if (!value) {
    return undefined;
  }
  const parsed = JSON.parse(value) as unknown;
  if (!isPlainObject(parsed)) {
    throw new Error(`${key} must be a JSON object`);
  }
  validateConfigValue(parsed, key);
  return parsed as CodexConfig;
}

function validateConfigValue(value: unknown, path: string): asserts value is CodexConfigValue {
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return;
  }
  if (Array.isArray(value)) {
    for (let index = 0; index < value.length; index += 1) {
      validateConfigValue(value[index], `${path}[${index}]`);
    }
    return;
  }
  if (isPlainObject(value)) {
    for (const [key, child] of Object.entries(value)) {
      validateConfigValue(child, `${path}.${key}`);
    }
    return;
  }
  throw new Error(`${path} contains an unsupported config value`);
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function defaultShell(): string {
  if (process.platform === "darwin") {
    return "/bin/zsh";
  }
  if (process.platform === "win32") {
    return process.env.ComSpec || "cmd.exe";
  }
  return "/bin/sh";
}

async function captureLoginShellEnv(shell: string, flags: string): Promise<Record<string, string>> {
  const marker = `___OPAGENT_CODEX_ENV_${Date.now().toString(36)}_${Math.random().toString(36).slice(2)}___`;
  const args = flags.trim() ? flags.trim().split(/\s+/) : ["-lic"];
  const { stdout } = await execFileAsync(shell, [...args, `echo '${marker}'; env`], {
    timeout: 15_000,
    maxBuffer: 2 * 1024 * 1024,
    env: process.env,
  });
  const idx = stdout.indexOf(marker);
  if (idx < 0) {
    throw new Error(`env marker not found in ${shell} output`);
  }
  const merged = envObject(process.env);
  for (const line of stdout.slice(idx + marker.length).split(/\r?\n/)) {
    const eq = line.indexOf("=");
    if (eq <= 0) {
      continue;
    }
    const key = line.slice(0, eq);
    if (/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) {
      merged[key] = line.slice(eq + 1);
    }
  }
  return merged;
}

function envObject(env: NodeJS.ProcessEnv): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(env)) {
    if (value !== undefined) {
      out[key] = value;
    }
  }
  return out;
}

export function platformName(): string {
  switch (os.platform()) {
    case "darwin":
      return "macOS";
    case "win32":
      return "Windows";
    case "linux":
      return "Linux";
    default:
      return os.platform();
  }
}
