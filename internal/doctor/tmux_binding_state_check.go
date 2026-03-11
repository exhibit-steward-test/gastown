package doctor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/tmux"
)

// TmuxBindingStateCheck validates that tmux cycle bindings are current and correctly configured.
// Detects:
//  1. Missing --client flag (causes multi-client navigation failures)
//  2. Stale prefix patterns (skips sessions from newly added rigs)
//  3. Missing GT bindings entirely
type TmuxBindingStateCheck struct {
	FixableCheck
}

// NewTmuxBindingStateCheck creates a new tmux binding state check.
func NewTmuxBindingStateCheck() *TmuxBindingStateCheck {
	return &TmuxBindingStateCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "tmux-binding-state",
				CheckDescription: "Validate tmux cycle bindings are current and correctly configured",
				CheckCategory:    CategoryInfrastructure,
			},
		},
	}
}

// Run checks that tmux cycle bindings (C-b n/p) are correctly configured.
func (c *TmuxBindingStateCheck) Run(ctx *CheckContext) *CheckResult {
	t := tmux.NewTmux()

	// Check if tmux server is running
	sessions, err := t.ListSessions()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not list tmux sessions",
			Details: []string{err.Error()},
		}
	}

	// If no sessions exist (nil or empty = no server), no bindings to check
	if len(sessions) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No tmux sessions (nothing to check)",
		}
	}

	// Check binding state
	hasBinding := c.hasGTBinding("prefix", "n")
	hasClient := c.hasClientFlag("prefix", "n")
	isCurrent := c.isCurrentPattern(ctx.TownRoot, "prefix", "n")

	var issues []string

	if !hasBinding {
		issues = append(issues, "Missing GT cycle bindings (C-b n/p) — keyboard navigation not configured")
	} else {
		if !hasClient {
			issues = append(issues, "Cycle bindings lack --client flag — multi-client navigation will fail")
		}
		if !isCurrent {
			issues = append(issues, "Cycle bindings have stale prefix pattern — newly added rig sessions unreachable")
		}
	}

	if len(issues) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Tmux cycle bindings correctly configured",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Found %d binding issue(s)", len(issues)),
		Details: issues,
		FixHint: "Run 'gt doctor --fix' to refresh cycle bindings",
	}
}

// Fix refreshes tmux cycle bindings on all existing sessions.
func (c *TmuxBindingStateCheck) Fix(ctx *CheckContext) error {
	t := tmux.NewTmux()

	sessions, err := t.ListSessions()
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		return nil // Nothing to fix if no sessions
	}

	// Refresh bindings on the first session (bindings are global, not per-session)
	// This will update the global key bindings for all sessions
	if err := t.SetCycleBindings(sessions[0]); err != nil {
		return fmt.Errorf("failed to refresh cycle bindings: %w", err)
	}

	return nil
}

// hasGTBinding checks if a GT binding exists for the given key.
func (c *TmuxBindingStateCheck) hasGTBinding(table, key string) bool {
	output, err := tmux.BuildCommand("list-keys", "-T", table, key).Output()
	if err != nil || len(output) == 0 {
		return false
	}
	outputStr := string(output)
	// GT bindings contain "if-shell" and "gt "
	return strings.Contains(outputStr, "if-shell") && strings.Contains(outputStr, "gt ")
}

// hasClientFlag checks if the binding includes --client for multi-client support.
func (c *TmuxBindingStateCheck) hasClientFlag(table, key string) bool {
	output, err := tmux.BuildCommand("list-keys", "-T", table, key).Output()
	if err != nil || len(output) == 0 {
		return false
	}
	return strings.Contains(string(output), "--client")
}

// isCurrentPattern checks if the binding has the current prefix pattern.
func (c *TmuxBindingStateCheck) isCurrentPattern(townRoot, table, key string) bool {
	output, err := tmux.BuildCommand("list-keys", "-T", table, key).Output()
	if err != nil || len(output) == 0 {
		return false
	}
	// Get the current expected pattern using the same logic as tmux.sessionPrefixPattern
	pattern := c.sessionPrefixPattern(townRoot)
	return strings.Contains(string(output), pattern)
}

// safePrefixRe matches the character set guaranteed by beadsPrefixRegexp.
var safePrefixRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,19}$`)

// sessionPrefixPattern returns the regex pattern for matching GT session prefixes.
// This duplicates the logic from internal/tmux/tmux.go for use in the doctor check.
func (c *TmuxBindingStateCheck) sessionPrefixPattern(townRoot string) string {
	seen := map[string]bool{"hq": true, "gt": true} // always include HQ + gastown fallback
	if townRoot != "" {
		for _, p := range config.AllRigPrefixes(townRoot) {
			if safePrefixRe.MatchString(p) {
				seen[p] = true
			}
		}
	}
	sorted := make([]string, 0, len(seen))
	for p := range seen {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)
	return "^(" + strings.Join(sorted, "|") + ")-"
}
