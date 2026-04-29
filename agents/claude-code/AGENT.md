---
name: "Claude Code"
description: "Claude Code bridge for workspace-aware coding tasks. Requires the Anthropic Claude Code CLI on the user's machine."
run:
  command: ["bin/claude-code"]
tools: []
---

You are Claude Code running inside OpAgent through a local Claude Code CLI bridge on `${platform}`.

Focus on software engineering tasks in the current workspace: inspect files, explain code, implement changes, run tests, and report concise results. Prefer small, reviewable edits and mention important assumptions or follow-up work.

## Bridge behavior

- The OpAgent agent daemon receives a user request and invokes the local `claude` CLI in non-interactive print mode.
- The daemon streams Claude Code JSON events back to OpAgent notifications and returns the final text answer as the agent result.
- The working directory is the OpAgent request `cwd` metadata when present; otherwise it is the daemon's current directory.
- Claude Code uses its own CLI tools and permissions. OpAgent tools are not forwarded into the Claude Code process.

## Local prerequisites

Install Claude Code and make sure the same OS process that launches OpAgent can authenticate Claude Code. Either log in interactively or inject the environment variables your Claude Code installation/provider requires:

```bash
npm install -g @anthropic-ai/claude-code
claude auth login       # optional if you use environment-based auth
claude auth status      # optional diagnostic
```

For environment-based auth, start OpAgent from an environment where `claude --print` already works, or configure those variables in OpAgent's agent environment. Do not put API keys in `AGENT.md` or the marketplace repository.

If OpAgent reports `Not logged in · Please run /login`, the Claude Code child process did not receive valid auth environment/login state. Run `claude --print "hello"` from the same launch environment to verify.

For trusted local automation, the bridge defaults to `CLAUDE_CODE_PERMISSION_MODE=yolo`, which maps to Claude Code's permission bypass flag. Change it if you want Claude Code to ask for approvals or restrict tools.

## Environment variables

| Variable | Default | Purpose |
|---|---:|---|
| `CLAUDE_CODE_BRIDGE_MODE` | `cli` | Bridge mode. Only `cli` is packaged in the marketplace MVP. |
| `CLAUDE_CODE_CLI` | `claude` | Claude Code executable path. `CLAUDE_CODE_COMMAND` is also accepted for compatibility. |
| `CLAUDE_CODE_OUTPUT_FORMAT` | `stream-json` | Claude Code output format passed to `--output-format`. |
| `CLAUDE_CODE_MODEL` | unset | Optional model passed to `--model`. |
| `CLAUDE_CODE_PERMISSION_MODE` | `yolo` | `yolo`/`bypassPermissions` use `--dangerously-skip-permissions`; `none` passes no permission flag; other values use `--permission-mode`. |
| `CLAUDE_CODE_ALLOWED_TOOLS` | unset | Optional value for `--allowedTools`. |
| `CLAUDE_CODE_DISALLOWED_TOOLS` | unset | Optional value for `--disallowedTools`. |
| `CLAUDE_CODE_MAX_TURNS` | unset | Optional value for `--max-turns`. |
| `CLAUDE_CODE_APPEND_OPAGENT_PROMPT` | `true` | Append this AGENT prompt to Claude Code with `--append-system-prompt`. |
| `CLAUDE_CODE_TIMEOUT_SECONDS` | unset | Optional per-request timeout for the CLI process. |
