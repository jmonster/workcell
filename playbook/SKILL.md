---
name: playbook
description: Turn a concrete recurring, replayable, or risky workflow into its smallest durable owner. Inspect existing codification first and remove or replace it when wrong. Do not use for ordinary implementation, safe one-offs, or imagined future reuse.
---

# Playbook

Use this only after real work exposes a workflow that has recurred, must be replayed or recovered, is risky to improvise, repeatedly needs the same judgment, or depends on a stable external capability. “Codify,” “document,” and “remember” do not qualify an otherwise ordinary task.

Find every command, skill, integration, and instruction that claims to own the workflow. Reuse it when correct, repair it when incomplete, and remove it when stale, misplaced, or duplicated. Do not preserve bad codification or create a parallel source of truth.

Choose the smallest owner:

- Deterministic steps: script or CLI.
- Recurring judgment or routing: skill.
- External capability: typed integration.
- Repository-wide invariant: `AGENTS.md`.
- Scarce shared resource: its existing coordinator.
- Safe or unproven one-off: nothing.

Finish the current task through the result. Keep executable behavior in code, preserve the task’s authorization boundaries, remove the superseded sequence, and stop. Reject wrappers that merely rename an existing command, prose that leaves the important steps to improvisation, and artifacts created only because the task was difficult.
