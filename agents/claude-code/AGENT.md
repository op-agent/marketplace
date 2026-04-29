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

Install and authenticate Claude Code before using this agent:

```bash
npm install -g @anthropic-ai/claude-code
claude auth login
claude auth status
```

If OpAgent reports `Not logged in · Please run /login`, run `claude auth login` as the same OS user that launches OpAgent, then restart/refresh the agent.

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
