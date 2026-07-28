# Workcell

Workcell gives cooperative processes a host-local FIFO lock for shared Macs, devices, GPUs, rigs, and license seats.

```bash
curl -fsSL https://workcell-137.pages.dev/install.sh | sh

workcell run macos-xcode --wait -- xcodebuild test
```

## Commands

```text
workcell run <resource> [--wait] [--session <id>] [--json] -- <command...>
workcell status <resource> [--json]
```

- `resource` is the lock and queue key.
- `--wait` joins the FIFO queue. Without it, a busy resource exits `75`.
- `--json` captures command output in `log_path` and emits one result on stdout.
- Busy JSON includes the owner, queue depth, and `next_action.wait_argv`.
- `--session` or `WORKCELL_SESSION` identifies the caller.

Resource names are exact coordination keys. Projects should establish one canonical name for each shared capability in their agent instructions; Workcell does not discover aliases.

## Agent contract

A busy result without duration history has this shape:

```json
{
  "schema_version": 1,
  "resource": "macos-xcode",
  "decision": "busy",
  "owner": {
    "reservation_id": "01K...",
    "session": "agent-a",
    "actor": "agent",
    "host": "build-mac",
    "pid": 7312,
    "repository": "workcell",
    "branch": "main",
    "cwd": "/src/workcell",
    "command": "xcodebuild test",
    "started_at": "2026-08-03T14:18:40Z",
    "elapsed_seconds": 12.418,
    "log_path": "/Users/runner/.local/state/workcell/logs/01K....log"
  },
  "queue_ahead": 0,
  "next_action": {
    "kind": "wait_or_continue",
    "wait_argv": [
      "/usr/local/bin/workcell", "run", "macos-xcode", "--wait",
      "--session", "agent-b", "--json", "--", "xcodebuild", "test"
    ]
  }
}
```

The agent can execute `wait_argv` unchanged, inspect the owner's log when relevant, or continue unrelated work. Completed results include the reservation, timings, log path, and wrapped command exit code.

State is stored under `~/.local/state/workcell`. Set `WORKCELL_STATE_DIR` to override it.

Workcell coordinates cooperative callers on one host. It is not authorization, preemption, discovery, scheduling, or distributed locking.

## Origin

Workcell grew out of projects such as Pixel Brite, where parallel agent tasks competed for a Mac and its simulators, an RTX 4090, and build hosts with little spare memory.

The first solution was a homegrown lease spread across a few scripts. It worked well enough that I had agents generalize the pattern into Workcell.

The tradeoff is cooperation: agents must follow the protocol, usually from a few lines in `AGENTS.md`. Those instructions can be lost during context compaction or ignored, but the approach has been reliable enough to become part of my normal local workflow.

## Development

```bash
make test
make demo
make multi-demo
```

The downloadable demo runs the released CLI and verifies FIFO handoff, exclusion, and independent resources:

```bash
curl -fsSL https://workcell-137.pages.dev/demo.sh | sh
```
