# Workcell

- Use Workcell only for a named resource that concurrent commands can corrupt, reset, replace, or validate against the wrong state. Ordinary builds, tests, files, and network access do not qualify by themselves.
- Prefer the checked-in command that owns the resource key. Never invent a key or wrap a command that already reserves its resources.
- A multi-resource wrapper must acquire each key once in bytewise order. Workcell rejects same-key recursion and order inversion.
- If a resource is busy, wait only when that result is still needed; otherwise do unrelated work. Do not poll, bypass the queue, or terminate its owner.
