---
name: Plan
description: Research a task and create or update a markdown plan file in .agent/plan with a dedicated task section.
tags: builtin
---

When this skill is selected, first inspect the relevant code and gather facts before writing the plan.

Rules:
- If runtime context includes planFilePath, you must write or update that exact file before you reply.
- Otherwise, if runtime context includes planDir, you must create the plan file inside that directory and choose a semantic markdown filename yourself.
- If the file already exists, update it instead of creating a second plan.
- If runtime context includes title, reuse it as the markdown heading or as frontmatter title unless you have a clearer concise plan title.
- The plan file must be ordinary markdown and must start with a markdown heading such as # Title.
- Include at least: a short goal or problem summary, and exactly one dedicated task section named ## Tasks or ## 任务.
- Only the dedicated task section may use task list items with - [ ] and - [x].
- Do not use - [ ] or - [x] anywhere else in the file, including facts, conclusions, risks, design notes, or acceptance criteria.
- Inside the dedicated task section, you may group work with ### subheadings.
- Do not wrap task list items in ordered list markers such as 1. or 2.
- Do not require yaml frontmatter in the plan file.
- Frontmatter is optional, but when you need metadata prefer simple scalar keys such as title.
- When planFilePath already exists, update that exact file instead of inventing a different filename.
- Keep the plan in the file, not in the chat reply.
- If you only describe the plan in chat without writing the bound file, that turn is invalid.
- Do not put the full research or final answer in the chat reply. The chat reply should only be a brief confirmation that the plan was created or updated.
