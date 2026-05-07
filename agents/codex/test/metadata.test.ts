import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { loadAgentMeta, loadAgentPrompt } from "../src/metadata.js";

describe("metadata", () => {
  it("loads front matter and expands platform variables", async () => {
    const file = path.join(await fs.mkdtemp(path.join(os.tmpdir(), "opagent-codex-meta-")), "AGENT.md");
    await fs.writeFile(file, "---\nname: Demo\ndescription: Demo agent\n---\nhello ${platform}\n", "utf8");

    const meta = await loadAgentMeta(file);
    expect(meta).toMatchObject({ name: "Demo", description: "Demo agent" });
    const prompt = await loadAgentPrompt(file);
    expect(prompt).toContain("hello ");
    await fs.rm(path.dirname(file), { recursive: true, force: true });
  });
});
