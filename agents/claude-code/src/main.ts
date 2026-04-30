import {
  OpCodes,
  OpServer,
  StdioTransport,
  textContent,
  type OpNodeParams,
  type OpNodeResult,
  type ServerRequest,
} from "@op-agent/opagent-protocol";
import { runClaudeAgent } from "./claude.js";
import { configFromEnv } from "./config.js";
import { loadAgentMeta, loadAgentPrompt, resolveAgentFile } from "./metadata.js";

async function main(): Promise<void> {
  const agentFile = await resolveAgentFile();
  const meta = await loadAgentMeta(agentFile);
  const cfg = configFromEnv();

  if (cfg.bridgeMode && cfg.bridgeMode !== "sdk") {
    throw new Error(`unsupported CLAUDE_CODE_BRIDGE_MODE=${JSON.stringify(cfg.bridgeMode)}; the TS agent supports "sdk"`);
  }

  const server = new OpServer({ name: meta.name, version: "0.2.1" });
  server.onOpNode((req) => handleOpNode(req, agentFile));
  server.addAgent(meta, async (req) => {
    const agentPrompt = cfg.appendOpAgentPrompt ? await loadAgentPrompt(agentFile) : "";
    return runClaudeAgent(req, cfg, agentPrompt);
  });
  await server.run(new StdioTransport());
}

async function handleOpNode(
  req: ServerRequest<OpNodeParams>,
  agentFile: string,
): Promise<OpNodeResult> {
  if (req.params.opCode !== OpCodes.PromptGet) {
    throw new Error(`unsupported opcode for claude-code: ${req.params.opCode}`);
  }
  return {
    opCode: req.params.opCode,
    _meta: { ...req.params._meta, agentID: "claude-code" },
    content: textContent(await loadAgentPrompt(agentFile)),
  };
}

main().catch((error: unknown) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
