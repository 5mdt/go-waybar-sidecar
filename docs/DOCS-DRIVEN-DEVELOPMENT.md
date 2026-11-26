# Docs-Driven Development Approach

A portable methodology for spec-first, AI-agent-friendly development. Copy
this file's structure into a new project and adapt the templates below.

## Core rules (put these in your project's CLAUDE.md / AGENTS.md)

- Documentation -> Tests -> Code, in that order.
- `docs/FRD.md` (and its children) is the source of truth. In case of
  conflict between docs and code/assumptions, the FRD wins.
- Keep documentation simple, short, and precise.
- Each feature doc must explain HOW a feature works. Reasons WHY should be
  at most one sentence.
- Reduce text churn: don't rewrite doc sections that didn't change.
- Prefer not increasing complexity (KISS, DRY, YAGNI).

## Directory layout

```
docs/
  FRD.md                  # index of all features, with a tag cross-reference
  todo.md                 # flat backlog of unpromoted ideas
  features/
    TEMPLATE.md           # empty skeleton for new feature docs
    EXAMPLE.md            # a filled-out reference doc
    <PREFIX>-NNNN-slug.md # one file per feature
```

- `<PREFIX>` is a short project code (e.g. `GWS`, `ABC`), `NNNN` is a
  zero-padded sequential ID. IDs are never reused or renumbered.
- One feature = one file. A feature is a user- or operator-visible behavior,
  not a code module.

## Workflow

1. **Write the doc first.** Copy `TEMPLATE.md` to
   `docs/features/<PREFIX>-NNNN-slug.md`, fill in Description and planned
   Implementation/Configuration. Add the entry (with tags) to `FRD.md`.
2. **Write tests against the doc's Testing section** (Human / Unit /
   Integration) before or alongside implementation.
3. **Implement** until tests pass and behavior matches the doc.
4. **Update Status** to `Implemented` (or `Planned` / `Deprecated`).
5. If a feature's behavior changes, edit its doc in place — don't create a
   new numbered doc for the same feature.
6. Ideas not yet worth a numbered feature go in `todo.md` as one line each;
   promote to a full doc when work starts.

## FRD.md template

```markdown
# Feature Requirements Document

## Available Features

- [<PREFIX>-0001. <Feature Name>](features/<PREFIX>-0001-slug.md) - `#tag1` `#tag2`

## Tags

- `#tag1`: <PREFIX>-0001, <PREFIX>-0003
- `#tag2`: <PREFIX>-0001
```

## Feature doc template (`docs/features/TEMPLATE.md`)

```markdown
# <id>. <Feature Name>

**Tags:** #tag1 #tag2

## Description

## Implementation

## Configuration

## Testing

### Human Testing

### Unit Testing

### Integration Testing

## Status
```

## Feature doc example (`docs/features/EXAMPLE.md`)

```markdown
# 8. Single-instance enforcement with takeover

**Tags:** #process-lifecycle

## Description

On startup, the sidecar checks for an already-running instance via its pidfile.
If one is found, the new process signals it to exit and takes over, instead of
exiting itself.

## Implementation

`pid.go` - `ensureSingleInstance()`:
- Reads `~/.local/state/waybar/waybar-sidecar.pid`.
- Verifies the PID belongs to a running `go-waybar-sidecar` process via
  `/proc/<pid>/exe`.
- Sends `SIGTERM` to the old process.
- Polls (`kill(pid, 0)`) every 100ms for up to 5s for it to exit.
- Proceeds with startup regardless of the outcome.

The old process handles `SIGTERM` in `main.go` (`run()`) and exits gracefully.

## Configuration

None.

## Testing

### Human Testing

- Start one instance, then start a second: the first should log a shutdown
  message and exit; the second should log "replacing running instance" and
  keep running.
- Verify `~/.local/state/waybar/waybar-sidecar.pid` ends up with the new PID.
- Start a second instance while the first is unresponsive (e.g. stopped with
  `SIGSTOP`): confirm the new instance logs a timeout warning after ~5s and
  still starts.

### Unit Testing

- `ensureSingleInstance()` with a pidfile pointing at a PID whose
  `/proc/<pid>/exe` does not end in `/go-waybar-sidecar`: should return
  immediately without signaling.
- `ensureSingleInstance()` with a pidfile containing the current process's own
  PID: should return immediately without signaling.
- `ensureSingleInstance()` with a missing or malformed pidfile: should return
  immediately.

### Integration Testing

- Launch the binary twice against a temp `pidDir`/config: assert the first
  process exits after receiving `SIGTERM` and the second remains running with
  its PID written to the pidfile.
- Launch a second instance while the first ignores `SIGTERM` (e.g. stopped via
  `SIGSTOP`): assert the second instance still starts after the ~5s timeout.

## Status

Implemented.
```

## todo.md template

```markdown
# Features to add
- <one-line idea>
```

## Adopting this in a new project

1. Create `docs/features/`, copy the two templates above into
   `TEMPLATE.md` and `EXAMPLE.md`.
2. Create `docs/FRD.md` with an empty `## Available Features` / `## Tags`.
3. Create `docs/todo.md`.
4. Add the Core rules section to the project's CLAUDE.md / AGENTS.md.
5. Pick a `<PREFIX>` and start numbering from `0001`.

## Known trade-offs

- The FRD index and tag table are hand-maintained — nothing enforces they
  stay in sync with the per-feature docs.
- Sequential IDs give no thematic grouping; tags are the only cross-cutting
  navigation.
- Works well up to roughly a few dozen features; a flat tag index gets
  unwieldy well beyond that.
- Relies on discipline (docs -> tests -> code) rather than tooling to
  enforce ordering.
