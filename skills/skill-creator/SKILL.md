---
name: Skill Creator
description: Create or update OpAgent skills for marketplace or local installation. Use this when the user wants to design, scaffold, improve, review, or package a SKILL.md, especially under ~/.opagent/skills, an agent-local skills directory, or the OpAgent marketplace.
tags: builtin
---

# Skill Creator

Use this skill when creating or improving an OpAgent skill. Think from the task outcome first, then write the smallest skill that reliably changes agent behavior.

## Workflow

1. Ground yourself in the current files before asking questions.
   - For a marketplace skill, inspect nearby `skills/<id>/SKILL.md` examples.
   - For a local skill, inspect `~/.opagent/skills/<id>/SKILL.md` if it exists.
   - For an agent-local skill, inspect the agent definition and its `skills/` directory.
2. Clarify only product intent that cannot be discovered from files:
   - What task should the skill make easier or more reliable?
   - When should it trigger?
   - What output or side effect should count as success?
   - Is it local-only, agent-local, marketplace, or builtin bundled with desktop/runtime?
3. Draft or update the skill with a lean `SKILL.md`.
4. Validate the skill against realistic prompts or the affected packaging path.
5. Keep the final response short and include changed paths plus validation results.

## Skill Layout

Use the standard OpAgent layout:

```text
skills/<skill-id>/
  SKILL.md
  references/   optional detailed docs loaded only when needed
  scripts/      optional deterministic helpers
  assets/       optional templates or media used by outputs
```

Local user skills install under:

```text
~/.opagent/skills/<skill-id>/SKILL.md
```

Agent-local skills live beside the agent:

```text
agents/<agent-id>/skills/<skill-id>/SKILL.md
```

Do not add README, changelog, install guide, or process notes unless the runtime actually needs them. Extra docs make skills harder for agents to use.

## SKILL.md Rules

Every skill must start with YAML frontmatter:

```yaml
---
name: Human Readable Name
description: What the skill does and when to use it. Include concrete trigger contexts.
---
```

Guidelines:

- Keep `name` stable once published.
- Make `description` action-oriented and specific; it is the trigger surface.
- Put trigger conditions in `description`, not only in the body.
- For marketplace builtin skills, add `tags: builtin`.
- Keep the body procedural and concise. Assume the agent is capable; add only context it would not already know.
- Prefer explicit steps and file layouts over vague advice.
- Use references for large domain details instead of bloating `SKILL.md`.
- Add scripts only when a repeated or fragile operation needs deterministic behavior.

## OpAgent Marketplace And Bundle Checks

For marketplace skills, keep the content public-safe:

- Do not include local absolute paths, private deployment hosts, private repository names, secrets, or release credentials.
- It is fine to reference public user-level paths such as `~/.opagent/skills/<id>/SKILL.md`.
- Keep the package root simple: `SKILL.md` must be at the root of the skill package.

If the skill is a builtin that must ship with desktop/runtime, update both surfaces:

- Marketplace catalog/build metadata so the online catalog publishes the skill.
- Desktop/runtime bundle staging and bootstrap logic so packaged installs sync it into `~/.opagent/skills/<id>/SKILL.md` without network access.

## Validation

Choose validation based on the change:

- New or edited marketplace skill: check frontmatter, package root, public-safety rules, and catalog metadata.
- Bundled builtin: confirm release staging includes `skills/<id>/SKILL.md` and local bootstrap syncs it.
- Behavior-heavy skill: test with 2-3 realistic user prompts and revise any instruction that causes confusion, overreach, or wasted work.

Prefer deleting unnecessary instructions over adding more rules. A good skill narrows the agent's path only where the task is fragile.
