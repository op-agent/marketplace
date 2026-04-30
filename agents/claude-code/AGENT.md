---
name: "Claude Code"
description: "Claude Code bridge for workspace-aware coding tasks. Requires the Anthropic Claude Code CLI on the user's machine."
opcodes:
  - thread/submit
  - prompt/get
run:
  command: ["bin/claude-code"]
  lifecycle: daemon
tools: []
---

You are Claude Code running inside OpAgent through the Claude Agent SDK on `${platform}`.

Focus on software engineering tasks in the current workspace: inspect files, explain code, implement changes, run tests, and report concise results. Prefer small, reviewable edits and mention important assumptions or follow-up work.

## Bridge behavior

- The OpAgent agent daemon is implemented in TypeScript and uses the official `@anthropic-ai/claude-agent-sdk`.
- By default the daemon captures environment variables from a login+interactive shell (`$SHELL -lic 'env'`) before starting the Claude Agent SDK, so shell-injected auth variables, PATH setup, and provider configuration are available.
- The daemon translates Claude SDK stream events into OpAgent notifications: text, thinking, tool calls, tool results, token usage, and final turn result.
- The working directory is the OpAgent request `cwd` metadata when present; otherwise it is the daemon's current directory.
- Claude Code uses its own tools, MCP configuration, sessions, and permissions. OpAgent tools are not forwarded into the Claude Code process.
- Claude session IDs are cached per OpAgent thread while the daemon is alive and reused with SDK `resume` on later turns.

## Local prerequisites

Install Claude Code and make sure the same OS process that launches OpAgent can authenticate Claude Code. Either log in interactively or inject the environment variables your Claude Code installation/provider requires:

```bash
npm install -g @anthropic-ai/claude-code
claude auth login       # optional if you use environment-based auth
claude auth status      # optional diagnostic
```

For environment-based auth, put the variables in a file loaded by your login or interactive shell, or start OpAgent from an environment where `claude --print` already works. The bridge captures the login+interactive shell environment by default so those variables can be injected into the Claude Code child process. Do not put API keys in `AGENT.md` or the marketplace repository.

If you need a different shell mode, set `CLAUDE_CODE_SHELL_FLAGS`; the default is `-lic` for zsh/bash login+interactive startup files. Because the bridge only captures `env` output and then runs Claude directly, shell startup banners should not corrupt Claude Code's JSON stream.

If OpAgent reports `Not logged in · Please run /login`, the Claude Code child process did not receive valid auth environment/login state. Run `claude --print "hello"` from the same launch environment to verify.

The bridge defaults to Claude Code's `default` permission mode. For trusted local automation, set `CLAUDE_CODE_PERMISSION_MODE=bypassPermissions` or `yolo`.

## Environment variables

| Variable | Default | Purpose |
|---|---:|---|
| `CLAUDE_CODE_BRIDGE_MODE` | `sdk` | Bridge mode. The TypeScript agent supports `sdk`. |
| `CLAUDE_CODE_CLI` | `claude` | Claude Code executable passed to the SDK. `CLAUDE_CODE_COMMAND` is also accepted for compatibility. |
| `CLAUDE_CODE_USE_LOGIN_SHELL` | `true` | Capture environment from a login+interactive shell before running Claude Code. Set `false` to use only the inherited process env. |
| `CLAUDE_CODE_SHELL` | `$SHELL` or platform default | Shell used when environment capture is enabled. |
| `CLAUDE_CODE_SHELL_FLAGS` | `-lic` | Shell flags used for environment capture. Must include `-c`; default covers login and interactive zsh/bash startup files. |
| `CLAUDE_CODE_MODEL` | unset | Optional model passed to `--model`. |
| `CLAUDE_CODE_PERMISSION_MODE` | `default` | SDK permission mode. `yolo` maps to `bypassPermissions`. |
| `CLAUDE_CODE_ALLOWED_TOOLS` | unset | Optional comma-separated tool allow list. |
| `CLAUDE_CODE_DISALLOWED_TOOLS` | unset | Optional comma-separated tool deny list. |
| `CLAUDE_CODE_MAX_TURNS` | unset | Optional max turns. |
| `CLAUDE_CODE_APPEND_OPAGENT_PROMPT` | `true` | Append this AGENT prompt to Claude Code's default system prompt. |
| `CLAUDE_CODE_INCLUDE_PARTIAL_MESSAGES` | `true` | Emit SDK partial stream events for token/tool lifecycle notifications. |
| `CLAUDE_CODE_RESUME_SESSIONS` | `true` | Resume Claude sessions by OpAgent thread while the daemon is alive. |
| `CLAUDE_CODE_TIMEOUT_SECONDS` | unset | Optional per-request timeout for the SDK query. |
