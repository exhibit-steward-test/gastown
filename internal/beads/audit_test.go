package beads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogPolecatLifecycle(t *testing.T) {
	// Create temporary directory for test beads
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("failed to create beads dir: %v", err)
	}

	// Create Beads instance
	b := &Beads{
		beadsDir: beadsDir,
	}

	// Test spawn event
	spawnEntry := PolecatLifecycleEntry{
		Timestamp:   "2024-03-19T10:00:00Z",
		Event:       "spawn",
		PolecatName: "toast",
		AgentBeadID: "greenplace/polecats/toast",
		State:       "spawning",
		Branch:      "polecat/toast-12345",
		BaseBranch:  "origin/main",
		HookBead:    "greenplace/work/GT-123",
	}

	if err := b.LogPolecatLifecycle(spawnEntry); err != nil {
		t.Fatalf("LogPolecatLifecycle spawn failed: %v", err)
	}

	// Test state_change event
	stateEntry := PolecatLifecycleEntry{
		Timestamp:   "2024-03-19T10:01:00Z",
		Event:       "state_change",
		PolecatName: "toast",
		AgentBeadID: "greenplace/polecats/toast",
		State:       "working",
		PriorState:  "spawning",
	}

	if err := b.LogPolecatLifecycle(stateEntry); err != nil {
		t.Fatalf("LogPolecatLifecycle state_change failed: %v", err)
	}

	// Test remove event
	removeEntry := PolecatLifecycleEntry{
		Timestamp:   "2024-03-19T10:02:00Z",
		Event:       "remove",
		PolecatName: "toast",
		AgentBeadID: "greenplace/polecats/toast",
		Reason:      "polecat removed",
	}

	if err := b.LogPolecatLifecycle(removeEntry); err != nil {
		t.Fatalf("LogPolecatLifecycle remove failed: %v", err)
	}

	// Test failure event
	failEntry := PolecatLifecycleEntry{
		Timestamp:   "2024-03-19T10:03:00Z",
		Event:       "failure",
		PolecatName: "broken",
		AgentBeadID: "greenplace/polecats/broken",
		State:       "spawn_failed",
		Error:       "git worktree creation failed",
	}

	if err := b.LogPolecatLifecycle(failEntry); err != nil {
		t.Fatalf("LogPolecatLifecycle failure failed: %v", err)
	}

	// Verify audit log was created and contains all entries
	auditPath := filepath.Join(beadsDir, "audit.log")
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("failed to read audit log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 audit entries, got %d", len(lines))
	}

	// Verify spawn entry structure
	var spawn PolecatLifecycleEntry
	if err := json.Unmarshal([]byte(lines[0]), &spawn); err != nil {
		t.Fatalf("failed to unmarshal spawn entry: %v", err)
	}
	if spawn.Event != "spawn" {
		t.Errorf("spawn.Event = %q, want %q", spawn.Event, "spawn")
	}
	if spawn.PolecatName != "toast" {
		t.Errorf("spawn.PolecatName = %q, want %q", spawn.PolecatName, "toast")
	}
	if spawn.State != "spawning" {
		t.Errorf("spawn.State = %q, want %q", spawn.State, "spawning")
	}
	if spawn.HookBead != "greenplace/work/GT-123" {
		t.Errorf("spawn.HookBead = %q, want %q", spawn.HookBead, "greenplace/work/GT-123")
	}

	// Verify state_change entry structure
	var stateChange PolecatLifecycleEntry
	if err := json.Unmarshal([]byte(lines[1]), &stateChange); err != nil {
		t.Fatalf("failed to unmarshal state_change entry: %v", err)
	}
	if stateChange.Event != "state_change" {
		t.Errorf("stateChange.Event = %q, want %q", stateChange.Event, "state_change")
	}
	if stateChange.State != "working" {
		t.Errorf("stateChange.State = %q, want %q", stateChange.State, "working")
	}
	if stateChange.PriorState != "spawning" {
		t.Errorf("stateChange.PriorState = %q, want %q", stateChange.PriorState, "spawning")
	}

	// Verify remove entry structure
	var remove PolecatLifecycleEntry
	if err := json.Unmarshal([]byte(lines[2]), &remove); err != nil {
		t.Fatalf("failed to unmarshal remove entry: %v", err)
	}
	if remove.Event != "remove" {
		t.Errorf("remove.Event = %q, want %q", remove.Event, "remove")
	}
	if remove.Reason != "polecat removed" {
		t.Errorf("remove.Reason = %q, want %q", remove.Reason, "polecat removed")
	}

	// Verify failure entry structure
	var failure PolecatLifecycleEntry
	if err := json.Unmarshal([]byte(lines[3]), &failure); err != nil {
		t.Fatalf("failed to unmarshal failure entry: %v", err)
	}
	if failure.Event != "failure" {
		t.Errorf("failure.Event = %q, want %q", failure.Event, "failure")
	}
	if failure.State != "spawn_failed" {
		t.Errorf("failure.State = %q, want %q", failure.State, "spawn_failed")
	}
	if !strings.Contains(failure.Error, "git worktree") {
		t.Errorf("failure.Error = %q, want to contain %q", failure.Error, "git worktree")
	}
}
