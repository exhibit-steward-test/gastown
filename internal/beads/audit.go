// Package beads provides audit logging for molecule operations.
package beads

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DetachAuditEntry represents an audit log entry for a detach operation.
type DetachAuditEntry struct {
	Timestamp        string `json:"timestamp"`
	Operation        string `json:"operation"` // "detach", "burn", "squash"
	PinnedBeadID     string `json:"pinned_bead_id"`
	DetachedMolecule string `json:"detached_molecule"`
	DetachedBy       string `json:"detached_by,omitempty"` // Agent that triggered detach
	Reason           string `json:"reason,omitempty"`      // Optional reason for detach
	PreviousState    string `json:"previous_state,omitempty"`
}

// PolecatLifecycleEntry represents an audit log entry for polecat lifecycle events.
// These events track the full lifecycle of worker agents from spawn to termination,
// enabling post-mortem analysis and debugging of agent behavior.
type PolecatLifecycleEntry struct {
	Timestamp   string `json:"timestamp"`
	Event       string `json:"event"`         // "spawn", "state_change", "remove", "failure"
	PolecatName string `json:"polecat_name"`  // Name of the polecat (e.g., "toast")
	AgentBeadID string `json:"agent_bead_id"` // Agent bead ID for this polecat
	State       string `json:"state,omitempty"` // Agent state: "spawning", "working", "done", "stuck"
	PriorState  string `json:"prior_state,omitempty"` // Previous state (for state_change events)
	Branch      string `json:"branch,omitempty"` // Git branch name
	BaseBranch  string `json:"base_branch,omitempty"` // Branch polecat was spawned from
	HookBead    string `json:"hook_bead,omitempty"` // Hook bead ID assigned at spawn
	Error       string `json:"error,omitempty"` // Error message (for failure events)
	Reason      string `json:"reason,omitempty"` // Optional reason/context
}

// DetachOptions specifies optional context for a detach operation.
type DetachOptions struct {
	Operation string // "detach", "burn", "squash" - defaults to "detach"
	Agent     string // Who is performing the detach
	Reason    string // Optional reason for the detach
}

// DetachMoleculeWithAudit removes molecule attachment from a pinned bead and logs the operation.
// Uses advisory file locking to prevent concurrent read-modify-write races.
// Returns the updated issue.
func (b *Beads) DetachMoleculeWithAudit(pinnedBeadID string, opts DetachOptions) (*Issue, error) {
	// Acquire per-bead lock to serialize concurrent attach/detach operations
	unlock, err := b.lockBead(pinnedBeadID)
	if err != nil {
		return nil, fmt.Errorf("acquiring bead lock: %w", err)
	}
	defer unlock()

	// Fetch the pinned bead first to get previous state
	issue, err := b.Show(pinnedBeadID)
	if err != nil {
		return nil, fmt.Errorf("fetching pinned bead: %w", err)
	}

	// Get current attachment info for audit
	attachment := ParseAttachmentFields(issue)
	if attachment == nil {
		return issue, nil // Nothing to detach
	}

	// Log the detach operation
	operation := opts.Operation
	if operation == "" {
		operation = "detach"
	}
	entry := DetachAuditEntry{
		Timestamp:        currentTimestamp(),
		Operation:        operation,
		PinnedBeadID:     pinnedBeadID,
		DetachedMolecule: attachment.AttachedMolecule,
		DetachedBy:       opts.Agent,
		Reason:           opts.Reason,
		PreviousState:    issue.Status,
	}
	if err := b.LogDetachAudit(entry); err != nil {
		// Log error but don't fail the detach operation
		fmt.Fprintf(os.Stderr, "Warning: failed to write audit log: %v\n", err)
	}

	// Clear attachment fields by passing nil
	newDesc := SetAttachmentFields(issue, nil)

	// Update the issue
	if err := b.Update(pinnedBeadID, UpdateOptions{Description: &newDesc}); err != nil {
		return nil, fmt.Errorf("updating pinned bead: %w", err)
	}

	// Re-fetch to return updated state
	return b.Show(pinnedBeadID)
}

// LogDetachAudit appends an audit entry to the audit log file.
// The audit log is stored in the resolved .beads directory as audit.log in JSONL format.
// This follows any beads redirect so audit entries go to the correct location.
func (b *Beads) LogDetachAudit(entry DetachAuditEntry) (retErr error) {
	return b.logAuditEntry(entry)
}

// LogPolecatLifecycle appends a polecat lifecycle audit entry to the audit log file.
// This records spawn, state changes, removal, and failures for worker agent observability.
// Non-fatal errors are logged to stderr but don't fail the operation.
func (b *Beads) LogPolecatLifecycle(entry PolecatLifecycleEntry) error {
	if err := b.logAuditEntry(entry); err != nil {
		// Log error but don't fail the lifecycle operation
		fmt.Fprintf(os.Stderr, "Warning: failed to write polecat lifecycle audit: %v\n", err)
		return err
	}
	return nil
}

// logAuditEntry is the internal implementation that writes any audit entry type to audit.log.
// The audit log is stored in the resolved .beads directory as audit.log in JSONL format.
// This follows any beads redirect so audit entries go to the correct location.
func (b *Beads) logAuditEntry(entry interface{}) (retErr error) {
	auditPath := filepath.Join(b.getResolvedBeadsDir(), "audit.log")

	// Marshal entry to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling audit entry: %w", err)
	}

	// Append to audit log file
	f, err := os.OpenFile(auditPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed internally
	if err != nil {
		return fmt.Errorf("opening audit log: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("closing audit log: %w", err)
		}
	}()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("writing audit entry: %w", err)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing audit log: %w", err)
	}

	return nil
}
