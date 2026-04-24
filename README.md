# OpAgent Marketplace

Official open-source catalog of agents, skills, and tools for [OpAgent](https://www.opagent.io).

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

## Build

Agent and tool binaries are compiled from Go source. They depend on private SDK packages (`github.com/op-agent/opagent-dev/packages/...`), so standalone compilation is not supported yet. Build automation lives in private internal tooling.

## Contributing

Changes to marketplace item definitions and source should be made in this repository.
