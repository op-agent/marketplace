# OpAgent Marketplace

Official catalog of agents, skills, and tools for [OpAgent](https://www.opagent.io).

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
| `systools` | System Tools | System tool server bundle used by the default agents |
| `rg-search` | RG Search | Ripgrep-based search tool for workspace and shell |

## Structure

```
agents/<id>/AGENT.md          Agent definition
skills/<id>/SKILL.md          Skill definition
tools/<id>/TOOLS.md           Tool definition
```

## Contributing

Changes to public marketplace item definitions should be made in this repository.
Binary build and release automation live in private internal tooling.
