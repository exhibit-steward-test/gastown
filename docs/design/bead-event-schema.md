# Bead Event Schema

> **Status**: Reference for instrumentation work
> **Updated**: 2026-03-20
> **Context**: Post-fa9dc28 hook-slot removal; schema standardization required

---

## Overview

This document defines the required and optional fields for bead event records written to `internal/beads/` (Dolt storage) and `internal/activity/` (activity tracking). All instrumentation work should reference this schema to ensure consistent, queryable event records across agent state changes, hook executions, polecat lifecycle transitions, and convoy stage progressions.

## Background: Why This Matters

Commit fa9dc28 ("Remove agent bead hook slot: use direct bead tracking") eliminated the `hook_bead` slot write/clear cycle in favor of direct bead tracking. This architectural shift means the schema is actively evolving. Codifying it now prevents:

1. **Schema drift**: Different subsystems emitting structurally incompatible beads
2. **Query fragility**: Inability to correlate events across agent/polecat/convoy boundaries
3. **Instrumentation gaps**: Missing fields discovered only when trying to debug past behavior

## Storage Locations

### 1. Dolt SQL Storage (`internal/beads/`)

**Primary table**: `issues` (schema version 6, see `docs/design/dolt-storage.md`)

Every bead is a row in the `issues` table. Event-like beads use:
- `issue_type`: Distinguishes events from tasks/messages/agents (e.g., `"event"`, `"agent"`)
- `description`: Structured key-value fields (see format below)
- `metadata`: JSON blob for extensible fields

**Audit trail table**: `events`
```sql
CREATE TABLE events (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    issue_id VARCHAR(255),
    event_type VARCHAR(64),  -- e.g., "agent_state_change", "hook_execution", "polecat_spawn"
    actor VARCHAR(255),      -- Who/what triggered this event
    old_value TEXT,          -- Previous state (JSON or plain text)
    new_value TEXT,          -- New state (JSON or plain text)
    created_at DATETIME
);
```

### 2. Activity Logs (`internal/activity/`)

Tracks last-activity timestamps for dashboard color-coding (green <5min, yellow 5-10min, red >10min).

**Package**: `internal/activity/activity.go`

**Usage**: Color-coded status display, not full event replay. For detailed history, query Dolt.

---

## Event Categories

Four primary event categories require instrumentation:

1. **Agent State Changes** — Agent lifecycle transitions (spawning → working → done → nuked)
2. **Hook Executions** — Hook dispatch, success, and failure events
3. **Polecat Lifecycle** — Polecat spawn, completion, failure, termination
4. **Convoy Stage Transitions** — Convoy progression (created → staged_ready → open → closed)

---

## 1. Agent State Change Events

**When to emit**: Any transition in `agent_state` field on agent beads (type=`agent`).

**Event type**: `"agent_state_change"`

### Required Fields

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `event_type` | string | Fixed: `"agent_state_change"` | `"agent_state_change"` |
| `issue_id` | string | Agent bead ID | `"gt-abc123"` |
| `actor` | string | Who/what triggered the change | `"witness"`, `"polecat/Toast"`, `"gt sling"` |
| `old_value` | string | Previous `agent_state` | `"idle"`, `"spawning"` |
| `new_value` | string | New `agent_state` | `"working"`, `"done"`, `"nuked"` |
| `created_at` | datetime | Event timestamp (RFC3339) | `"2026-03-20T10:15:30Z"` |

### Agent State Values

Defined in `internal/beads/beads_agent.go` (`AgentFields.AgentState`):

- `"spawning"` — Agent session is being created
- `"working"` — Actively executing assigned work
- `"done"` — Completed work, awaiting cleanup
- `"stuck"` — Agent explicitly signaled need for help
- `"escalated"` — Work escalated to human/higher authority
- `"idle"` — Session killed, identity preserved for reuse
- `"running"` — Generic active state (used by non-polecat agents)
- `"nuked"` — Session terminated, sandbox cleaned, identity persists

### Optional Contextual Fields

| Field | Type | Description |
|-------|------|-------------|
| `role_type` | string | `"polecat"`, `"witness"`, `"refinery"`, `"deacon"`, `"mayor"` |
| `rig` | string | Rig name (empty for global agents) |
| `hook_bead` | string | Currently pinned work bead ID (if transitioning in/out of work) |
| `cleanup_status` | string | Git state: `"clean"`, `"has_uncommitted"`, `"has_stash"`, `"has_unpushed"` |
| `active_mr` | string | Merge request bead ID (for traceability) |
| `exit_type` | string | Completion type: `"COMPLETED"`, `"ESCALATED"`, `"DEFERRED"`, `"PHASE_COMPLETE"` |

### Example Event Record

```json
{
  "event_type": "agent_state_change",
  "issue_id": "gt-abc123",
  "actor": "gt done",
  "old_value": "working",
  "new_value": "done",
  "created_at": "2026-03-20T10:15:30Z",
  "role_type": "polecat",
  "rig": "greenplace",
  "hook_bead": "gt-def456",
  "exit_type": "COMPLETED"
}
```

---

## 2. Hook Execution Events

**When to emit**: Hook dispatch start, completion (success), and failure.

**Event types**:
- `"hook_start"` — Hook dispatch begins
- `"hook_success"` — Hook completed successfully
- `"hook_failure"` — Hook execution failed

### Required Fields

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `event_type` | string | `"hook_start"`, `"hook_success"`, `"hook_failure"` | `"hook_success"` |
| `issue_id` | string | Work bead ID (the bead being worked on) | `"gt-xyz789"` |
| `actor` | string | Agent executing the hook | `"polecat/Toast"` |
| `hook_name` | string | Hook identifier | `"on_edit"`, `"on_task_complete"` |
| `created_at` | datetime | Event timestamp (RFC3339) | `"2026-03-20T10:20:00Z"` |

### Optional Contextual Fields

| Field | Type | Description |
|-------|------|-------------|
| `hook_config` | JSON | Hook configuration snapshot (for replay) |
| `error` | string | Failure reason (for `hook_failure` only) |
| `duration_ms` | int | Execution duration in milliseconds (for `hook_success`/`hook_failure`) |
| `exit_code` | int | Process exit code (if applicable) |

### Hook Centralization Note

**Post-fa9dc28 architecture**: Hook instrumentation should be centralized in a single dispatch chokepoint (likely `internal/cmd/hookutil` or similar, to be established). All hooks must inherit coverage from shared dispatch logic—no hook type should bypass instrumentation.

### Example Event Record

```json
{
  "event_type": "hook_success",
  "issue_id": "gt-xyz789",
  "actor": "polecat/Toast",
  "hook_name": "on_edit",
  "created_at": "2026-03-20T10:20:05Z",
  "duration_ms": 1200
}
```

---

## 3. Polecat Lifecycle Events

**When to emit**: Polecat spawn, state transitions, completion, termination.

**Event types**:
- `"polecat_spawn"` — New polecat session started
- `"polecat_state_change"` — Polecat state transition
- `"polecat_complete"` — Polecat finished work successfully
- `"polecat_failure"` — Polecat session failed/crashed
- `"polecat_terminate"` — Polecat session explicitly killed

### Required Fields

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `event_type` | string | Event type (see above) | `"polecat_spawn"` |
| `issue_id` | string | Polecat agent bead ID | `"gt-pc-toast"` |
| `actor` | string | Who/what triggered the event | `"witness"`, `"gt sling"`, `"polecat/Toast"` |
| `old_value` | string | Previous polecat state (null for spawn) | `"idle"`, `"working"` |
| `new_value` | string | New polecat state | `"working"`, `"done"`, `"zombie"` |
| `created_at` | datetime | Event timestamp (RFC3339) | `"2026-03-20T10:25:00Z"` |

### Polecat State Values

Defined in `internal/polecat/types.go` (`State` type):

- `"working"` — Session active, doing assigned work (normal operation)
- `"idle"` — Work completed, session killed, sandbox preserved for reuse
- `"done"` — Called `gt done`, transient state before exit
- `"stuck"` — Explicitly signaled need for assistance
- `"zombie"` — Tmux session exists but worktree is missing (orphaned)

Note: "Stalled" is a *detected condition* (Witness monitors tmux/age), not a stored state.

### Optional Contextual Fields

| Field | Type | Description |
|-------|------|-------------|
| `rig` | string | Rig name |
| `polecat_name` | string | Polecat identifier (e.g., `"Toast"`) |
| `hook_bead` | string | Assigned work bead ID |
| `branch` | string | Git branch name |
| `clone_path` | string | Worktree path |
| `cleanup_status` | string | Git state: `"clean"`, `"has_uncommitted"`, `"has_stash"`, `"has_unpushed"` |
| `exit_type` | string | Completion type (for `polecat_complete`) |
| `error` | string | Failure reason (for `polecat_failure`) |

### Example Event Record

```json
{
  "event_type": "polecat_spawn",
  "issue_id": "gt-pc-toast",
  "actor": "gt sling",
  "old_value": null,
  "new_value": "working",
  "created_at": "2026-03-20T10:25:00Z",
  "rig": "greenplace",
  "polecat_name": "Toast",
  "hook_bead": "gt-work123",
  "branch": "feature/fix-parser"
}
```

---

## 4. Convoy Stage Transition Events

**When to emit**: Convoy status changes (created → staged → open → closed/aborted).

**Event type**: `"convoy_stage_transition"`

### Required Fields

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `event_type` | string | Fixed: `"convoy_stage_transition"` | `"convoy_stage_transition"` |
| `issue_id` | string | Convoy bead ID | `"gt-convoy-abc"` |
| `actor` | string | Who/what triggered the transition | `"witness"`, `"gt convoy launch"` |
| `old_value` | string | Previous convoy status | `"staged_ready"`, `"open"` |
| `new_value` | string | New convoy status | `"open"`, `"closed"` |
| `created_at` | datetime | Event timestamp (RFC3339) | `"2026-03-20T10:30:00Z"` |

### Convoy Status Values

Defined in `internal/cmd/convoy.go` (status constants):

- `"open"` — Convoy actively coordinating work
- `"closed"` — Convoy completed or cancelled
- `"staged_ready"` — Pre-launch staging completed, ready to launch
- `"staged_warnings"` — Staging completed with warnings (may still launch)

Additional internal states (not user-facing):
- `"created"` — Initial bead creation
- `"stranded"` — Detected condition: convoy has stalled tasks
- `"aborted"` — Explicitly terminated before completion

### Valid Transitions

From `validateConvoyStatusTransition` in `internal/cmd/convoy.go`:

- `open` ↔ `closed` (standard lifecycle)
- `staged_ready` → `open` (launch)
- `staged_ready` → `closed` (cancel before launch)
- `staged_warnings` → `open` (launch despite warnings)
- `staged_warnings` → `closed` (cancel)
- `staged_*` ↔ `staged_*` (re-stage with different result)

**Rejected transitions**: `open` → `staged_*`, `closed` → `staged_*`

### Optional Contextual Fields

| Field | Type | Description |
|-------|------|-------------|
| `molecule` | string | Molecule type (if convoy is molecule-based) |
| `epic_id` | string | Parent epic bead ID (if convoy tracks epic) |
| `task_count` | int | Number of tasks in convoy |
| `wave` | int | Current dispatch wave number |
| `merge_strategy` | string | Merge strategy for convoy branches |
| `reason` | string | Transition reason (especially for `closed`/`aborted`) |

### Example Event Record

```json
{
  "event_type": "convoy_stage_transition",
  "issue_id": "gt-convoy-abc",
  "actor": "gt convoy launch",
  "old_value": "staged_ready",
  "new_value": "open",
  "created_at": "2026-03-20T10:30:00Z",
  "epic_id": "gt-epic-xyz",
  "task_count": 5,
  "wave": 1,
  "merge_strategy": "queue"
}
```

---

## Implementation Guidance

### 1. Use Existing Bead Primitives

All events should use the established `beads` package primitives:

```go
import "github.com/steveyegge/gastown/internal/beads"

// For structured agent fields
fields := &beads.AgentFields{
    RoleType:   "polecat",
    AgentState: "working",
    // ...
}

// For creating event records in Dolt
// (Exact API TBD - this is conceptual)
```

### 2. Field Format for `description`

Agent beads store structured fields as `key: value` lines in the `description` field. Example from `beads_agent.go`:

```
Polecat Toast

role_type: polecat
rig: greenplace
agent_state: working
hook_bead: gt-abc123
cleanup_status: clean
```

Use `null` for empty optional fields to distinguish "not set" from "set to empty string".

### 3. Timestamp Format

All timestamps must use RFC3339 format for consistency:

```go
time.Now().Format(time.RFC3339)
// "2026-03-20T10:15:30Z"
```

### 4. Actor Naming Conventions

| Actor Type | Format | Example |
|------------|--------|---------|
| CLI command | `"gt <subcommand>"` | `"gt sling"`, `"gt done"` |
| Agent role | `"<role>/<name>"` | `"polecat/Toast"`, `"witness"` |
| System process | `"<subsystem>"` | `"scheduler"`, `"daemon"` |
| User | `"user:<username>"` | `"user:steve"` |

### 5. Telemetry Integration

Bead events should correlate with OpenTelemetry spans for distributed tracing:

```go
import "github.com/steveyegge/gastown/internal/telemetry"

// Wrap operations with OTel spans
ctx, span := telemetry.StartSpan(ctx, "polecat.spawn")
defer span.End()

// Emit bead event with same context
// ...
```

See `docs/design/otel/otel-data-model.md` for OTel span conventions.

---

## Testing Requirements

Any PR implementing new instrumentation must include:

1. **Unit tests**: Verify event record structure and required fields
2. **Integration tests**: Assert bead records appear in Dolt after operations
   - Use `testcontainers-go` + Dolt module (already in `go.mod`)
   - Exercise polecat spawn-to-completion, convoy stage advance, hook execution
   - Query `events` table and verify event records exist
3. **OTel span coverage**: Verify corresponding trace spans exist for event-emitting operations

---

## Migration Notes

### Post-fa9dc28 Cleanup

The `hook_bead` field still exists in `AgentFields` struct (`internal/beads/beads_agent.go`) but is no longer maintained via dedicated set/clear methods. It's written as part of `FormatAgentDescription` but the slot write/clear cycle is gone.

**Action for instrumentation work**: Confirm whether `hook_bead` should:
- Be fully removed from `AgentFields` (breaking change)
- Remain as read-only legacy field for historical beads
- Be repurposed as direct work assignment tracking (current state)

### Hook Instrumentation Centralization

Before fa9dc28, hook tracking used a dedicated `hook_bead` slot. Now, hook execution should emit direct event records. The audit of fa9dc28 (backlog item 519cc708) will enumerate former hook-slot call sites and ensure each has a replacement event emission.

**Centralization target**: All hook dispatch should flow through a single instrumentation chokepoint (to be established, likely in `internal/cmd/` or a new `internal/hookutil/` package). This ensures:
- No hook type can be added without inheriting coverage
- Consistent event schema across all hook executions
- Single place to add cross-cutting concerns (rate limiting, circuit breakers, etc.)

---

## Open Questions

1. **Granularity**: Should crew member role transitions (idle → working → blocked → handoff) be separate event types, or fold into `agent_state_change`?
   - **Recommendation**: Use `agent_state_change` with `role_type="crew"` and extend state vocabulary if needed.

2. **Retention**: Should ephemeral events (e.g., heartbeats) use wisps (Dolt-ignored) or regular beads?
   - **Recommendation**: High-frequency events (<1min intervals) should use wisps to avoid Dolt commit spam. Lifecycle transitions (spawn, complete) should use regular beads for auditability.

3. **Schema evolution**: How to handle breaking changes (e.g., renaming `agent_state` values)?
   - **Recommendation**: Follow Dolt migration pattern (add `schema_version` to metadata, support N and N-1).

---

## References

- **Dolt Schema**: `docs/design/dolt-storage.md` — Full schema definition
- **Agent Fields**: `internal/beads/beads_agent.go` — `AgentFields` struct and formatting
- **Polecat States**: `internal/polecat/types.go` — `State` type and lifecycle documentation
- **Convoy Statuses**: `internal/cmd/convoy.go` — Status constants and transition validation
- **Activity Tracking**: `internal/activity/activity.go` — Last-activity color coding
- **OTel Data Model**: `docs/design/otel/otel-data-model.md` — Trace span conventions
- **Commit fa9dc28**: "Remove agent bead hook slot: use direct bead tracking"

---

## Changelog

| Date | Change | Author |
|------|--------|--------|
| 2026-03-20 | Initial schema documentation (post-fa9dc28) | Steward: Instrumentation Beads Monitor |
