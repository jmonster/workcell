# Playbook

Playbook helps a coding agent decide whether proven work needs a durable owner. It can reuse or repair an existing command, remove stale instructions, create a narrower interface, or leave a safe one-off alone.

## Install

```bash
curl -fsSL https://workcell-137.pages.dev/playbook/install.sh | sh
```

Then invoke it after a workflow has recurred, must be replayed or recovered, is risky to improvise, repeatedly needs the same judgment, or depends on a stable external capability:

```text
Use $playbook to decide what should persist from this task, then complete the task through it.
```

## Decisions

| Work discovered | Durable result |
| --- | --- |
| Correct existing authority | Reuse it |
| Incomplete existing authority | Repair it |
| Stale or duplicate authority | Remove it |
| Deterministic steps | Script or CLI |
| Recurring judgment | Skill |
| External capability | Typed integration |
| Repository-wide invariant | `AGENTS.md` |
| Safe or unproven one-off | Nothing |

The current task must finish through the result. A missing wrapper, a difficult task, or possible future reuse is not enough.

The canonical skill is [`SKILL.md`](SKILL.md). Codex interface metadata is in [`agents/openai.yaml`](agents/openai.yaml).
