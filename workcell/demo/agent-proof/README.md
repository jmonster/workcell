# Real-agent false-green proof

Three fresh Codex agents validate three isolated Workcell candidates against one shared integration-binary slot: `wait-regression`, `baseline`, and `baseline-replica`.

`wait-regression` contains a seeded `--wait` defect. Its first validation should fail; both baselines should pass.

Without Workcell, deploy-and-test windows overlap. Another agent replaces the regression binary before its test, producing a false green. An independent test later finds the unfixed regression.

With Workcell, each deploy-and-test window holds the shared resource. `wait-regression` tests its own binary, fixes the defect outside the critical section, then queues for another attempt.

Run it in a terminal:

```bash
./demo/agent-proof/run.sh
```

The dashboard has three windows: control, Workcell, and final scorecard. Proof events update the current window in place. It never changes windows on its own. Press `←` to go back and `→` to advance after the current arm completes.

- The control shows overlapping validations, at least one false green, and fewer than three correct candidates.
- Workcell shows one owner, two queued agents, no false greens, and three correct candidates.

The command exits nonzero unless those conditions are observed.

Use `--plain` only for CI or log capture; it disables the dashboard.
