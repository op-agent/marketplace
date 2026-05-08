# OpAgent Marketplace

Open-source catalog of agents, skills, and tools for [OpAgent](https://www.opagent.io).

## Agents

| ID | Name | Description |
|---|---|---|
| `claude-code` | Claude Code | Claude Code bridge for workspace-aware coding tasks |
| `codex` | Codex | OpenAI Codex SDK bridge for workspace-aware coding tasks |
| `completion` | Completion | Inline editor completion prompt |
| `opagent` | OpAgent | Expert coding assistant for development and debugging |
| `researcher` | Researcher | Evidence-first research agent for cited markdown reports |

## Skills

| ID | Name | Description |
|---|---|---|
| `plan` | Plan | Research a task and create or update a bound markdown plan file |
| `execute-plan` | Execute Plan | Execute a bound markdown plan file and update checklist progress |
| `skill-creator` | Skill Creator | Create or update OpAgent skills for marketplace or local installation |
| `agent-browser-search` | Agent Browser Search | Search the web with agent-browser and extract source evidence |
| `chatgpt-image` | ChatGPT Image | Binary-only skill for paid OpAgent image generation and editing |
| `hyperframes` | HyperFrames | Create HyperFrames HTML video compositions, animations, captions, audio-reactive visuals, and transitions |
| `hyperframes-cli` | HyperFrames CLI | Use the HyperFrames CLI for scaffold, lint, inspect, preview, render, and environment troubleshooting |
| `hyperframes-media` | HyperFrames Media | Preprocess assets for HyperFrames with TTS, transcription, and background removal workflows |
| `hyperframes-registry` | HyperFrames Registry | Install and wire HyperFrames registry blocks and components into compositions |
| `tailwind` | Tailwind | Use Tailwind CSS v4 browser-runtime patterns in HyperFrames compositions |
| `gsap` | GSAP | Write deterministic GSAP animations and timelines for HyperFrames compositions |
| `animejs` | Anime.js | Write deterministic Anime.js animations and timelines for HyperFrames compositions |
| `css-animations` | CSS Animations | Write deterministic CSS keyframe animations for HyperFrames compositions |
| `lottie` | Lottie | Embed deterministic Lottie and dotLottie animations in HyperFrames compositions |
| `three` | Three.js | Create deterministic Three.js and WebGL scenes for HyperFrames compositions |
| `waapi` | Web Animations API | Write deterministic Web Animations API motion for HyperFrames compositions |
| `website-to-hyperframes` | Website to HyperFrames | Capture a website and turn it into a HyperFrames video workflow |
| `remotion-to-hyperframes` | Remotion to HyperFrames | Translate explicit Remotion migration requests into HyperFrames HTML compositions |

## Tools

| ID | Name | Description |
|---|---|---|
| `rg-search` | RG Search | Ripgrep-based search tool for workspace and shell |

## Structure

```
agents/<id>/
  AGENT.md              Agent definition
  cmd/<id>/main.go      Agent entry point, when the agent has a runnable daemon
  src/                  TypeScript agent source, when the agent is SDK-backed
  bin/                  Agent launcher scripts or compiled binaries
  skills/               Agent-specific skills (if any)
  tools/                Agent-specific tools (if any)

skills/<id>/
  SKILL.md              Skill definition

closed-packages/<id>/
  manifest.json         Binary-only package metadata
  <platform>.tar.gz     Prebuilt package archive uploaded to the catalog bucket

tools/<id>/
  TOOLS.md              Tool definition
  cmd/main.go           Tool entry point
```

Helper code should live beside the agent or tool that uses it. Do not add shared `internal/` packages unless the code is intentionally becoming a public reusable module.

## Publishing

Pull requests run public-safe validation only. After changes are merged to `main`, GitHub Actions builds the marketplace packages and publishes the latest catalog to R2.

Required publishing secrets are stored in GitHub repository settings, not in this repository.

## Build

This repository is a standalone Go module with a few TypeScript daemon agents.

```bash
go test ./...
cd agents/claude-code && npm run check
cd ../codex && npm run check
```

GitHub Actions builds release packages for supported platforms and publishes them to R2 after changes are merged to `main`.

## Contributing

Changes to marketplace item definitions and source should be made in this repository through pull requests.
