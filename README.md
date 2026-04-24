# OpAgent Marketplace

Open-source catalog of agents, skills, and tools for [OpAgent](https://www.opagent.io).

## Agents

| ID | Name | Description |
|---|---|---|
| `opagent` | OpAgent | Expert coding assistant for development and debugging |
| `researcher` | Researcher | Evidence-first research agent for cited markdown reports |

## Skills

| ID | Name | Description |
|---|---|---|
| `plan` | Plan | Research a task and create or update a bound markdown plan file |
| `execute-plan` | Execute Plan | Execute a bound markdown plan file and update checklist progress |

## Tools

| ID | Name | Description |
|---|---|---|
| `rg-search` | RG Search | Ripgrep-based search tool for workspace and shell |

## Structure

```
agents/<id>/
  AGENT.md              Agent definition
  cmd/<id>/main.go      Agent entry point
  skills/               Agent-specific skills (if any)
  tools/                Agent-specific tools (if any)

skills/<id>/
  SKILL.md              Skill definition

tools/<id>/
  TOOLS.md              Tool definition
  cmd/main.go           Tool entry point
```

Helper code should live beside the agent or tool that uses it. Do not add shared `internal/` packages unless the code is intentionally becoming a public reusable module.

## Publishing

Pull requests run public-safe validation only. After changes are merged to `main`, GitHub Actions builds the marketplace packages and publishes the latest catalog to R2.

Required publishing secrets are stored in GitHub repository settings, not in this repository.

## Build

This repository is a standalone Go module.

```bash
go test ./...
```

GitHub Actions builds release packages for supported platforms and publishes them to R2 after changes are merged to `main`.

## Contributing

Changes to marketplace item definitions and source should be made in this repository through pull requests.
