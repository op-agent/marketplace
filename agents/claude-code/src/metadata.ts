import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { AgentMeta } from "@op-agent/opagent-protocol";
import { platformName } from "./config.js";

const agentID = "claude-code";

export async function resolveAgentFile(env: NodeJS.ProcessEnv = process.env): Promise<string> {
  const explicit = env.OPAGENT_AGENT_FILE?.trim();
  if (explicit && await fileExists(explicit)) {
    return explicit;
  }

  const candidates: string[] = [];
  const moduleDir = path.dirname(fileURLToPath(import.meta.url));
  candidates.push(
    path.join(moduleDir, "..", "AGENT.md"),
    path.join(moduleDir, "..", "..", "AGENT.md"),
  );

  const cwd = process.cwd();
  candidates.push(
    path.join(cwd, ".agent", "AGENT.md"),
    path.join(cwd, "AGENT.md"),
  );
  for (let dir = cwd; ; dir = path.dirname(dir)) {
    candidates.push(path.join(dir, "agents", agentID, "AGENT.md"));
    const parent = path.dirname(dir);
    if (parent === dir) {
      break;
    }
  }

  for (const candidate of candidates) {
    const resolved = path.resolve(candidate);
    if (await fileExists(resolved)) {
      return resolved;
    }
  }
  throw new Error(`AGENT.md not found for ${agentID}`);
}

export async function loadAgentMeta(agentFile: string): Promise<AgentMeta> {
  const raw = await fs.readFile(agentFile, "utf8");
  const front = parseFrontMatter(raw);
  return {
    name: front.name || "Claude Code",
    description: front.description || "Claude Code bridge for workspace-aware coding tasks.",
  };
}

export async function loadAgentPrompt(agentFile: string): Promise<string> {
  const raw = await fs.readFile(agentFile, "utf8");
  return expandPromptVariables(markdownBody(raw));
}

export function markdownBody(markdown: string): string {
  const split = splitFrontMatter(markdown);
  return (split?.body ?? markdown).trim();
}

export function parseFrontMatter(markdown: string): { name?: string; description?: string } {
  const split = splitFrontMatter(markdown);
  if (!split) {
    return {};
  }
  const out: { name?: string; description?: string } = {};
  for (const line of split.frontMatter.split(/\r?\n/)) {
    const match = /^([A-Za-z0-9_-]+)\s*:\s*(.*)$/.exec(line);
    if (!match) {
      continue;
    }
    const key = match[1];
    const value = unquote(match[2].trim());
    if (key === "name") {
      out.name = value;
    } else if (key === "description") {
      out.description = value;
    }
  }
  return out;
}

function splitFrontMatter(markdown: string): { frontMatter: string; body: string } | null {
  const text = markdown.replace(/^\uFEFF/, "").replace(/\r\n/g, "\n");
  if (!text.startsWith("---\n")) {
    return null;
  }
  const remaining = text.slice("---\n".length);
  const idx = remaining.indexOf("\n---");
  if (idx < 0) {
    return null;
  }
  let bodyStart = idx + "\n---".length;
  if (remaining[bodyStart] === "\n") {
    bodyStart += 1;
  }
  return {
    frontMatter: remaining.slice(0, idx).trim(),
    body: remaining.slice(bodyStart),
  };
}

function expandPromptVariables(prompt: string): string {
  return prompt
    .replaceAll("${platform}", platformName())
    .replaceAll("{{platform}}", platformName());
}

function unquote(value: string): string {
  if ((value.startsWith("\"") && value.endsWith("\"")) || (value.startsWith("'") && value.endsWith("'"))) {
    return value.slice(1, -1);
  }
  return value;
}

async function fileExists(file: string): Promise<boolean> {
  try {
    const stat = await fs.stat(file);
    return stat.isFile();
  } catch {
    return false;
  }
}
