# Playbook

Playbook is a coding-agent skill for turning work proven during a real task into the smallest durable interface that should outlive it.

## Install and use

```bash
curl -fsSL https://workcell-137.pages.dev/playbook/install.sh | sh
```

Or tell your coding agent:

```text
Install the Playbook skill using https://workcell-137.pages.dev/playbook/llms.txt
```

Then invoke it after a workflow has qualified:

```text
Use $playbook to decide what should persist from this task, then complete the task through it.
```

## Worked example: Stringproof

Apple `.strings` files can still parse after translation while losing a key or placeholder, gaining prompt residue, or mixing scripts. Given source and target catalogs, those checks are deterministic. Under Playbook, they belong in a CLI rather than another skill.

[Stringproof](../stringproof/README.md) is that durable interface. Its fixtures make the result runnable from the repository:

```bash
python3 site/public/downloads/stringproof.pyz check stringproof/fixtures/corrupted --source-locale en
python3 site/public/downloads/stringproof.pyz check stringproof/fixtures/clean --source-locale en
```

The corrupted fixture reports findings and exits `1`; the clean fixture prints nothing and exits `0`. The same command supports stable JSON for agents and CI.

## When work qualifies

Work should persist when at least one of these is true:

- The procedure has already recurred.
- Another operator must replay or recover it exactly.
- Improvised or reordered steps carry material risk.
- Agents repeatedly need the same judgment or routing.
- A stable external capability needs a reusable interface.

A missing wrapper, a difficult task, or possible future reuse is not enough. A safe one-off should remain a one-off.

## Where it belongs

| Work discovered | Durable result |
| --- | --- |
| An existing command or instruction already owns it | Extend that authority |
| Repeatable steps with explicit inputs and outputs | Script or CLI |
| Recurring judgment, routing, or tool selection | Skill |
| Stable external service or data source | MCP server or integration |
| Repository-wide invariant | `AGENTS.md` |
| Scarce or stateful resource | Existing coordinator |
| Safe, disposable, or unproven work | Nothing |

Playbook rejects a parallel wrapper when Make, a package task, or the repository's CLI already owns the job.

## Acceptance test

The current task must finish through the interface the agent creates or changes. That forces the result to handle the real inputs, failures, permissions, and repository constraints. If the task still depends on remembering the transcript, the work was not codified.

## Limits

Playbook does not expand the task or its permissions. It does not turn ordinary implementation into infrastructure, and it does not preserve a workflow merely because it may be useful someday.

The canonical skill is [`SKILL.md`](SKILL.md). Codex interface metadata lives in [`agents/openai.yaml`](agents/openai.yaml).
