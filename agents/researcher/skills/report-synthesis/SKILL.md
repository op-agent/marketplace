---
name: Report Synthesis
description: Use this skill to turn collected evidence into a consulting-style Markdown report with explicit structure, citations, caveats, and a concise executive summary.
---

# Report Synthesis Skill

Use this skill after the evidence has been collected.

## Deliverable

Write a complete Markdown report to:
- `reports/<threadID>/<YYYYMMDD>-<slug>.md` when `threadID` is available
- otherwise `reports/general/<YYYYMMDD>-<slug>.md`

Then reply in chat with:
- a short executive summary
- major caveats or confidence limits
- the report path

## Report Structure

Use this shape unless the user requested a different format:

1. Title
2. Executive Summary
3. Scope and Method
4. Key Findings
5. Supporting Evidence
6. Risks, Unknowns, or Conflicts
7. Conclusion
8. Sources

## Writing Rules

- Be explicit about what is known vs inferred.
- If evidence conflicts, show the conflict.
- Do not fabricate numbers or quotes.
- Use inline citations near the claim they support.
- Prefer compact, readable Markdown over decorative formatting.

## Analytical Style

- Start from evidence, then interpretation.
- For recommendations or conclusions, explain why the evidence supports them.
- Highlight missing data rather than hiding it.
