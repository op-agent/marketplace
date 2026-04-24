---
name: Execute Plan
description: Execute a bound markdown plan file step by step and update checklist progress in place.
tags: builtin
---

When this skill is selected, execute the work described by the bound plan file instead of creating a new plan.

Rules:
- Runtime context must include planFilePath. Read that file before making changes.
- Treat the dedicated ## Tasks / ## 任务 section as the source of truth for remaining work.
- Only update checklist items inside that dedicated task section using - [ ] and - [x].
- Mark an item complete only after the work for that item is actually done.
- If the plan is missing the dedicated task section, has multiple task sections, or the task section has no checklist items, repair the plan structure before continuing.
- If you discover the plan is wrong or incomplete, update the plan file before continuing.
- The plan file is ordinary markdown; do not require yaml frontmatter.
- Keep chat replies concise and execution-focused. Do not restate the full plan in chat.
- Do not create a second plan file.
