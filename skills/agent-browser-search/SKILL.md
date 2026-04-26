---
name: Agent Browser Search
description: Search the web with agent-browser and extract source evidence from live pages. Use this when the user asks to search, browse, inspect search results, compare pages, or collect web evidence with agent-browser. Run agent-browser --help for current usage and install guidance before relying on commands.
tags: builtin
---

# Agent Browser Search

Use `agent-browser` for browser-backed web search and source inspection. Prefer it when the user wants live web results, pages that need JavaScript, screenshots, or interactive follow-up through search results.

## Setup

Check whether the CLI exists:

```bash
command -v agent-browser
agent-browser --help
```

If it is missing, install one of these ways:

```bash
npm install -g agent-browser
agent-browser install
```

```bash
brew install agent-browser
agent-browser install
```

```bash
cargo install agent-browser
agent-browser install
```

On Linux, use:

```bash
agent-browser install --with-deps
```

If browser setup looks broken:

```bash
agent-browser doctor --fix
```

For version-matched command details, load the CLI's own guide:

```bash
agent-browser skills get core
agent-browser skills get core --full
```

## Search Workflow

1. Start with the current CLI help so flags match the installed version:

   ```bash
   agent-browser --help
   ```

2. Open a search engine and search the query:

   ```bash
   agent-browser open https://duckduckgo.com
   agent-browser snapshot -i
   agent-browser find label "Search" fill "search query"
   agent-browser press Enter
   agent-browser wait --load networkidle
   agent-browser snapshot -i --urls
   ```

3. Treat result snippets as leads, not evidence. Open promising results and inspect the source page:

   ```bash
   agent-browser click @e5 --new-tab
   agent-browser wait --load networkidle
   agent-browser get title
   agent-browser get url
   agent-browser snapshot -i -c
   ```

4. Extract only the relevant text from the source page:

   ```bash
   agent-browser get text "body"
   ```

5. Repeat across enough independent sources to answer confidently. Preserve title, URL, publisher, and visible date when available.

6. Close the browser when finished:

   ```bash
   agent-browser close
   ```

## Natural Language Search

If `agent-browser chat` is configured in the environment, it can run the browser flow directly:

```bash
agent-browser -q chat "Search the web for <query>, open the best sources, and summarize the answer with source URLs."
```

Use this for broad exploration. For precise citations or high-stakes facts, still inspect the source pages yourself with `snapshot`, `get title`, `get url`, and targeted text extraction.

## Output Expectations

- Answer from opened source pages, not from search snippets alone.
- Include source URLs when the user asks for evidence, freshness, or verification.
- State when a page could not be opened, required login, blocked automation, or returned ambiguous results.
- Do not enter credentials or personal data unless the user explicitly asks and provides a safe auth method.
