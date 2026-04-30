import { execFile } from "node:child_process";
import os from "node:os";
import { promisify } from "node:util";
import type { Options, PermissionMode } from "@anthropic-ai/claude-agent-sdk";

const execFileAsync = promisify(execFile);

export interface ClaudeCodeConfig {
  bridgeMode: string;
  model?: string;
  permissionMode?: PermissionMode;
  allowedTools?: string[];
  disallowedTools?: string[];
  maxTurns?: number;
  appendOpAgentPrompt: boolean;
  notifyRawEvents: boolean;
  includePartialMessages: boolean;
  useLoginShell: boolean;
  shell: string;
  shellFlags: string;
  timeoutMs?: number;
  pathToClaudeCodeExecutable?: string;
  resumeSessions: boolean;
}

export function configFromEnv(env: NodeJS.ProcessEnv = process.env): ClaudeCodeConfig {
  const permissionMode = normalizePermissionMode(firstEnv(env, "CLAUDE_CODE_PERMISSION_MODE"));
  return {
    bridgeMode: firstEnv(env, "CLAUDE_CODE_BRIDGE_MODE") || "sdk",
    model: firstEnv(env, "CLAUDE_CODE_MODEL") || undefined,
    permissionMode,
    allowedTools: csvEnv(env, "CLAUDE_CODE_ALLOWED_TOOLS"),
    disallowedTools: csvEnv(env, "CLAUDE_CODE_DISALLOWED_TOOLS"),
    maxTurns: positiveInt(firstEnv(env, "CLAUDE_CODE_MAX_TURNS")),
    appendOpAgentPrompt: boolEnv(env, "CLAUDE_CODE_APPEND_OPAGENT_PROMPT", true),
    notifyRawEvents: boolEnv(env, "CLAUDE_CODE_NOTIFY_RAW_EVENTS", false),
    includePartialMessages: boolEnv(env, "CLAUDE_CODE_INCLUDE_PARTIAL_MESSAGES", true),
    useLoginShell: boolEnv(env, "CLAUDE_CODE_USE_LOGIN_SHELL", true),
    shell: firstEnv(env, "CLAUDE_CODE_SHELL", "SHELL") || defaultShell(),
    shellFlags: firstEnv(env, "CLAUDE_CODE_SHELL_FLAGS") || "-lic",
    timeoutMs: positiveInt(firstEnv(env, "CLAUDE_CODE_TIMEOUT_SECONDS")) != null
      ? positiveInt(firstEnv(env, "CLAUDE_CODE_TIMEOUT_SECONDS"))! * 1000
      : undefined,
    pathToClaudeCodeExecutable: firstEnv(env, "CLAUDE_CODE_CLI", "CLAUDE_CODE_COMMAND") || "claude",
    resumeSessions: boolEnv(env, "CLAUDE_CODE_RESUME_SESSIONS", true),
  };
}

export async function buildClaudeOptions(
  cfg: ClaudeCodeConfig,
  params: {
    cwd?: string;
    agentPrompt?: string;
    resume?: string;
    abortController: AbortController;
  },
): Promise<Options> {
  const env = cfg.useLoginShell
    ? await captureLoginShellEnv(cfg.shell, cfg.shellFlags).catch((error: unknown) => {
      console.error(`claude-code: login shell env capture failed: ${error instanceof Error ? error.message : String(error)}`);
      return process.env;
    })
    : process.env;

  return {
    abortController: params.abortController,
    cwd: params.cwd,
    env: envObject(env),
    model: cfg.model,
    allowedTools: cfg.allowedTools,
    disallowedTools: cfg.disallowedTools,
    maxTurns: cfg.maxTurns,
    resume: params.resume,
    includePartialMessages: cfg.includePartialMessages,
    pathToClaudeCodeExecutable: cfg.pathToClaudeCodeExecutable,
    permissionMode: cfg.permissionMode,
    allowDangerouslySkipPermissions: cfg.permissionMode === "bypassPermissions" || undefined,
    systemPrompt: cfg.appendOpAgentPrompt && params.agentPrompt?.trim()
      ? { type: "preset", preset: "claude_code", append: params.agentPrompt }
      : undefined,
  };
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
  const value = env[key]?.trim().toLowerCase();
  if (!value) {
    return fallback;
  }
  return ["1", "true", "yes", "y", "on"].includes(value)
    ? true
    : ["0", "false", "no", "n", "off"].includes(value)
      ? false
      : fallback;
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

function normalizePermissionMode(value: string): PermissionMode | undefined {
  const normalized = value.trim().toLowerCase();
  if (!normalized || normalized === "none" || normalized === "off" || normalized === "false") {
    return "default";
  }
  if (["yolo", "bypass", "bypasspermissions", "bypass-permissions", "dangerously-skip-permissions", "skip"].includes(normalized)) {
    return "bypassPermissions";
  }
  if (normalized === "acceptedits" || normalized === "accept-edits") {
    return "acceptEdits";
  }
  if (["default", "plan", "dontask", "auto"].includes(normalized)) {
    return normalized === "dontask" ? "dontAsk" : normalized as PermissionMode;
  }
  return value as PermissionMode;
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

async function captureLoginShellEnv(shell: string, flags: string): Promise<NodeJS.ProcessEnv> {
  const marker = `___OPAGENT_ENV_${Date.now().toString(36)}_${Math.random().toString(36).slice(2)}___`;
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
  const merged: NodeJS.ProcessEnv = { ...process.env };
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

function envObject(env: NodeJS.ProcessEnv): Record<string, string | undefined> {
  return { ...env, CLAUDE_AGENT_SDK_CLIENT_APP: env.CLAUDE_AGENT_SDK_CLIENT_APP || "opagent-claude-code/0.2.1" };
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
