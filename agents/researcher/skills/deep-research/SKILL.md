---
name: Deep Research
description: Use this skill for multi-source online research before answering or writing a report. It drives broad discovery, targeted source collection, browser fallback, and evidence verification.
---

# Deep Research Skill

Use this skill whenever the task requires current information, comparisons, market scans, background research, or any report that depends on external sources.

## Workflow

1. Clarify missing scope first.
2. Start with `web_search` for broad discovery.
3. Use `web_fetch` to read the most relevant sources in full.
4. If pages are JS-heavy, blocked, or incomplete, switch to `browser_search` or `browser_fetch`.
5. Search from multiple angles instead of relying on one query.
6. Cross-check important claims across multiple sources.
7. Distinguish:
   - verified fact
   - source claim
   - your inference

## Minimum Research Bar

Before writing the final report, make sure you have:
- multiple sources, not one
- at least one primary or authoritative source when available
- explicit dates for time-sensitive claims
- contradictory evidence called out when it exists

## Search Strategy

- Begin broad, then narrow.
- Try multiple query phrasings.
- Prefer official docs, company statements, filings, or reputable publications over summaries.
- Use browser fallback only when API or direct fetch is insufficient.

## Output Rules

- Keep notes concise during collection.
- Put the final detail in the Markdown report file, not the chat reply.
- Every major external claim in the report must have an inline citation: `[citation:Title](URL)`.
