package doctor

import (
	"testing"
)

// TestTmuxBindingStateCheck_NoSessions verifies the check passes when no sessions exist.
func TestTmuxBindingStateCheck_NoSessions(t *testing.T) {
	check := NewTmuxBindingStateCheck()
	ctx := &CheckContext{
		TownRoot: "/tmp/gastown-test",
	}

	result := check.Run(ctx)

	// When no tmux server is running, we expect either:
	// - StatusOK with "No tmux sessions" (if tmux returns empty list)
	// - StatusWarning with "Could not list tmux sessions" (if tmux not installed/running)
	// Both are acceptable for this test
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("Expected StatusOK or StatusWarning, got %v: %s", result.Status, result.Message)
	}
}

// TestSessionPrefixPattern verifies the pattern generation matches expected format.
func TestSessionPrefixPattern(t *testing.T) {
	check := NewTmuxBindingStateCheck()

	// Test with empty town root (should only have hq and gt)
	pattern := check.sessionPrefixPattern("")
	if pattern != "^(gt|hq)-" {
		t.Errorf("Expected '^(gt|hq)-' for empty town root, got %q", pattern)
	}
}

// TestSafePrefixRe verifies the prefix validation regex.
func TestSafePrefixRe(t *testing.T) {
	tests := []struct {
		prefix string
		valid  bool
	}{
		{"gt", true},
		{"hq", true},
		{"my-rig", true},
		{"MyRig123", true},
		{"a", true}, // Single letter is valid
		{"1invalid", false}, // Must start with letter
		{"has space", false},
		{"has$pecial", false},
		{"", false},
		{"a123456789012345678901234567890", false}, // Too long (>20 chars)
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			matches := safePrefixRe.MatchString(tt.prefix)
			if matches != tt.valid {
				t.Errorf("Expected safePrefixRe.MatchString(%q) = %v, got %v", tt.prefix, tt.valid, matches)
			}
		})
	}
}
