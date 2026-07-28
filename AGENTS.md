## Shared capability protocol

Commands using a scarce, physical, licensed, or stateful capability must run through Workcell.

Examples include macOS build hosts, physical devices, GPUs, hardware rigs, FPGA labs, and limited license seats.

Resource keys are canonical project vocabulary. Use the key already assigned to a capability; do not invent an alias for it.

Before running such a command:

1. Invoke `workcell run <resource> --json -- <command...>`.
2. If the decision is `busy`, do not bypass Workcell or terminate the owner.
3. Use the returned `wait_argv` when waiting is appropriate. Otherwise continue work that does not require that capability.
4. Inspect another session's log only when it is relevant to coordination.
5. In the completion report, include the resource, session, reservation ID, exit code, and log path.

Workcell coordinates cooperative agents on one host; it does not provide preemption or distributed locking.
