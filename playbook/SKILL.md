---
name: playbook
description: Turn a workflow discovered during real work into the smallest durable interface that should outlive the task. Use when a procedure has recurred, must be replayable or recoverable, carries material risk when improvised, or the user explicitly asks to make agent work reusable. Do not use for safe one-off diagnostics, ordinary implementation, imagined future reuse, or work an existing checked-in authority already handles unchanged.
---

# Playbook

Leave the current task with less agent-only knowledge than it started with.

## Decide whether the work should persist

Inspect the repository, its instructions, and its existing commands before creating anything. Codify only when at least one of these is true:

- The procedure has already recurred.
- Another operator must replay or recover it exactly.
- Skipped, reordered, or improvised steps carry material risk.
- Agents repeatedly need the same domain judgment or routing.
- The work exposes a stable external capability worth reusing.

A missing wrapper and possible future reuse are not evidence. Keep a safe one-off as a one-off.

## Reuse authority first

Find the command, skill, integration, or instruction that already owns the work. Extend that surface when its contract can honestly cover the new case. Never create a parallel entry point that only renames or redispatches an existing one.

## Choose the durable form

- **Existing authority:** Extend it when the work belongs there.
- **Script or CLI:** Use for deterministic local steps with explicit inputs, outputs, exit status, and no model judgment.
- **Skill:** Use for recurring judgment, routing, constraints, or selection among existing tools.
- **MCP server or integration:** Use when a stable external capability or data source needs a typed tool boundary.
- **Project rule:** Use `AGENTS.md` for a repository-wide invariant that must shape ordinary work.
- **Coordinator:** Route scarce or stateful resources through the coordinator that already owns them.
- **Nothing:** Leave safe, disposable, or unproven work alone.

## Implement through the result

1. Define one narrow contract: trigger, inputs, outputs, failure states, and non-goals.
2. Put enforcement in deterministic code where possible. Skills route and preserve judgment; they do not duplicate executable behavior in prose.
3. Complete the current requested outcome through the interface you leave behind. If it cannot handle the real case, it is not the artifact.
4. Remove or stop using the superseded agent-only sequence. Preserve authorization boundaries; making work reusable does not authorize broader work.
5. Stop after the smallest real interface works. Do not add a framework, compatibility path, report, or speculative feature.

## Reject false extractions

Reject an artifact that:

- Only aliases Make, package scripts, or the repository's canonical CLI.
- Documents steps without owning or routing them.
- Requires an agent to improvise its important invariants.
- Exists only because the current task was difficult.
- Creates another source of truth.

The acceptance test is the current task, performed through the interface that will remain.
