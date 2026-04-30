import { describe, expect, it } from "vitest";
import { markdownBody, parseFrontMatter } from "../src/metadata.js";

describe("metadata", () => {
  it("parses front matter and body", () => {
    const markdown = "---\nname: \"Claude Code\"\ndescription: Test agent\n---\nhello ${platform}\n";
    expect(parseFrontMatter(markdown)).toEqual({
      name: "Claude Code",
      description: "Test agent",
    });
    expect(markdownBody(markdown)).toBe("hello ${platform}");
  });
});
