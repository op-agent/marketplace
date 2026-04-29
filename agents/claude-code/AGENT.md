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

You are Claude Code running inside OpAgent through a local Claude Code CLI bridge on `${platform}`.

Focus on software engineering tasks in the current workspace: inspect files, explain code, implement changes, run tests, and report concise results. Prefer small, reviewable edits and mention important assumptions or follow-up work.

## Bridge behavior

- The OpAgent agent daemon receives a user request and invokes the local `claude` CLI in non-interactive print mode.
- By default the daemon captures environment variables from a login+interactive shell (`$SHELL -lic 'env'`) and then runs Claude Code directly, so shell-injected auth variables, PATH setup, and provider configuration are available without shell startup output corrupting Claude's JSON stream.
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

For environment-based auth, put the variables in a file loaded by your login or interactive shell, or start OpAgent from an environment where `claude --print` already works. The bridge captures the login+interactive shell environment by default so those variables can be injected into the Claude Code child process. Do not put API keys in `AGENT.md` or the marketplace repository.

If you need a different shell mode, set `CLAUDE_CODE_SHELL_FLAGS`; the default is `-lic` for zsh/bash login+interactive startup files. Because the bridge only captures `env` output and then runs Claude directly, shell startup banners should not corrupt Claude Code's JSON stream.

If OpAgent reports `Not logged in · Please run /login`, the Claude Code child process did not receive valid auth environment/login state. Run `claude --print "hello"` from the same launch environment to verify.

For trusted local automation, the bridge defaults to `CLAUDE_CODE_PERMISSION_MODE=yolo`, which maps to Claude Code's permission bypass flag. Change it if you want Claude Code to ask for approvals or restrict tools.

## Environment variables

| Variable | Default | Purpose |
|---|---:|---|
| `CLAUDE_CODE_BRIDGE_MODE` | `cli` | Bridge mode. Only `cli` is packaged in the marketplace MVP. |
| `CLAUDE_CODE_CLI` | `claude` | Claude Code executable path. `CLAUDE_CODE_COMMAND` is also accepted for compatibility. |
| `CLAUDE_CODE_USE_LOGIN_SHELL` | `true` | Capture environment from a login+interactive shell before running Claude Code. Set `false` to use only the inherited process env. |
| `CLAUDE_CODE_SHELL` | `$SHELL` or platform default | Shell used when environment capture is enabled. |
| `CLAUDE_CODE_SHELL_FLAGS` | `-lic` | Shell flags used for environment capture. Must include `-c`; default covers login and interactive zsh/bash startup files. |
| `CLAUDE_CODE_OUTPUT_FORMAT` | `stream-json` | Claude Code output format passed to `--output-format`. |
| `CLAUDE_CODE_MODEL` | unset | Optional model passed to `--model`. |
| `CLAUDE_CODE_PERMISSION_MODE` | `yolo` | `yolo`/`bypassPermissions` use `--dangerously-skip-permissions`; `none` passes no permission flag; other values use `--permission-mode`. |
| `CLAUDE_CODE_ALLOWED_TOOLS` | unset | Optional value for `--allowedTools`. |
| `CLAUDE_CODE_DISALLOWED_TOOLS` | unset | Optional value for `--disallowedTools`. |
| `CLAUDE_CODE_MAX_TURNS` | unset | Optional value for `--max-turns`. |
| `CLAUDE_CODE_APPEND_OPAGENT_PROMPT` | `true` | Append this AGENT prompt to Claude Code with `--append-system-prompt`. |
| `CLAUDE_CODE_TIMEOUT_SECONDS` | unset | Optional per-request timeout for the CLI process. |
