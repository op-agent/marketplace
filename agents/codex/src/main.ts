import {
  OpCodes,
  OpServer,
  StdioTransport,
  textContent,
  type OpNodeParams,
  type OpNodeResult,
  type ServerRequest,
} from "@op-agent/opagent-protocol";
import { runCodexAgent } from "./codex.js";
import { configFromEnv } from "./config.js";
import { loadAgentMeta, loadAgentPrompt, resolveAgentFile } from "./metadata.js";

const agentID = "codex";

async function main(): Promise<void> {
  const agentFile = await resolveAgentFile();
  const meta = await loadAgentMeta(agentFile);
  const cfg = configFromEnv();

  const server = new OpServer({ name: meta.name, version: "0.1.0" });
  server.onOpNode((req) => handleOpNode(req, agentFile));
  server.addAgent(meta, async (req) => {
    const agentPrompt = cfg.appendOpAgentPrompt ? await loadAgentPrompt(agentFile) : "";
    return runCodexAgent(req, cfg, agentPrompt);
  }, { id: agentID, aliases: ["Codex"] });
  await server.run(new StdioTransport());
}

async function handleOpNode(
  req: ServerRequest<OpNodeParams>,
  agentFile: string,
): Promise<OpNodeResult> {
  if (req.params.opCode !== OpCodes.PromptGet) {
    throw new Error(`unsupported opcode for codex: ${req.params.opCode}`);
  }
  return {
    opCode: req.params.opCode,
    _meta: { ...req.params._meta, agentID },
    content: textContent(await loadAgentPrompt(agentFile)),
  };
}

main().catch((error: unknown) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
