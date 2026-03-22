// Package compat provides cross-runtime hook compatibility testing.
//
// This test suite ensures behavioral parity across all Gas Town runtime adapters
// (Claude Code, Cursor, Gemini, and future runtimes). Every hook scenario defined
// here must produce identical outcomes across all runtimes.
//
// Design rationale:
// The IsAutonomousRole extraction (commit 76ef3fa) proved that logic was silently
// duplicated across four packages. Without a cross-runtime test matrix, the next
// divergence won't be caught until a guard works on Claude but silently no-ops on
// Cursor. This test suite is the single highest-leverage structural investment for
// maintaining hook parity.
package compat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/cursor"
	"github.com/steveyegge/gastown/internal/gemini"
)

// HookScenario defines a cross-runtime hook test case.
type HookScenario struct {
	Name        string
	Description string
	RoleType    string // "autonomous" or "interactive"
	Assertions  []HookAssertion
}

// HookAssertion defines expected behavior for a specific hook.
type HookAssertion struct {
	HookName        string   // e.g., "SessionStart", "PreToolUse", "Stop"
	ExpectedCommand string   // Command that should be present
	Matcher         string   // Tool matcher if applicable (empty for unconditional hooks)
	MustBePresent   bool     // Whether this hook must exist
	CommandContains []string // Substrings that must appear in the command
}

// RuntimeHookConfig represents parsed hook configuration for any runtime.
type RuntimeHookConfig struct {
	Hooks map[string][]HookEntry
}

// HookEntry represents a single hook entry.
type HookEntry struct {
	Command string
	Matcher string
}

// TestAutonomousRoleHookParity verifies that autonomous roles have identical
// hook behavior across all runtimes.
func TestAutonomousRoleHookParity(t *testing.T) {
	scenario := HookScenario{
		Name:        "Autonomous Role Hooks",
		Description: "Autonomous roles (polecat, witness, refinery) need identical startup and shutdown hooks",
		RoleType:    "autonomous",
		Assertions: []HookAssertion{
			{
				HookName:        "SessionStart",
				MustBePresent:   true,
				CommandContains: []string{"gt prime --hook", "gt mail check --inject"},
			},
			{
				HookName:        "PreCompact",
				MustBePresent:   true,
				CommandContains: []string{"gt prime --hook"},
			},
			{
				HookName:        "Stop",
				MustBePresent:   true,
				CommandContains: []string{"gt costs record"},
			},
			{
				HookName:        "PreToolUse",
				Matcher:         "gh pr create",
				MustBePresent:   true,
				CommandContains: []string{"gt tap guard pr-workflow"},
			},
			{
				HookName:        "PreToolUse",
				Matcher:         "git checkout -b",
				MustBePresent:   true,
				CommandContains: []string{"gt tap guard pr-workflow"},
			},
			{
				HookName:        "PreToolUse",
				Matcher:         "git switch -c",
				MustBePresent:   true,
				CommandContains: []string{"gt tap guard pr-workflow"},
			},
		},
	}

	testScenarioAcrossRuntimes(t, scenario)
}

// TestInteractiveRoleHookParity verifies that interactive roles have identical
// hook behavior across all runtimes.
func TestInteractiveRoleHookParity(t *testing.T) {
	scenario := HookScenario{
		Name:        "Interactive Role Hooks",
		Description: "Interactive roles (mayor, crew) need identical user-prompt and shutdown hooks",
		RoleType:    "interactive",
		Assertions: []HookAssertion{
			{
				HookName:        "UserPromptSubmit",
				MustBePresent:   true,
				CommandContains: []string{"gt mail check --inject"},
			},
			{
				HookName:        "PreCompact",
				MustBePresent:   true,
				CommandContains: []string{"gt prime --hook"},
			},
			{
				HookName:        "Stop",
				MustBePresent:   true,
				CommandContains: []string{"gt costs record"},
			},
			{
				HookName:        "PreToolUse",
				Matcher:         "gh pr create",
				MustBePresent:   true,
				CommandContains: []string{"gt tap guard pr-workflow"},
			},
		},
	}

	testScenarioAcrossRuntimes(t, scenario)
}

// TestRoleClassificationParity verifies that all runtimes use the same
// IsAutonomousRole logic from hookutil.
func TestRoleClassificationParity(t *testing.T) {
	testRoles := []struct {
		role       string
		autonomous bool
	}{
		{"polecat", true},
		{"witness", true},
		{"refinery", true},
		{"deacon", true},
		{"boot", true},
		{"mayor", false},
		{"crew", false},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range testRoles {
		t.Run(tt.role, func(t *testing.T) {
			claudeType := claude.RoleTypeFor(tt.role)
			cursorType := cursor.RoleTypeFor(tt.role)
			geminiType := gemini.RoleTypeFor(tt.role)

			expectedType := "interactive"
			if tt.autonomous {
				expectedType = "autonomous"
			}

			if string(claudeType) != expectedType {
				t.Errorf("Claude: RoleTypeFor(%q) = %q, want %q", tt.role, claudeType, expectedType)
			}
			if string(cursorType) != expectedType {
				t.Errorf("Cursor: RoleTypeFor(%q) = %q, want %q", tt.role, cursorType, expectedType)
			}
			if string(geminiType) != expectedType {
				t.Errorf("Gemini: RoleTypeFor(%q) = %q, want %q", tt.role, geminiType, expectedType)
			}

			// All three must agree
			if string(claudeType) != string(cursorType) || string(claudeType) != string(geminiType) {
				t.Errorf("Role classification divergence for %q: Claude=%q, Cursor=%q, Gemini=%q",
					tt.role, claudeType, cursorType, geminiType)
			}
		})
	}
}

// testScenarioAcrossRuntimes executes a hook scenario against all runtime adapters.
func testScenarioAcrossRuntimes(t *testing.T, scenario HookScenario) {
	// Test Claude
	t.Run("Claude", func(t *testing.T) {
		config := generateClaudeConfig(t, scenario.RoleType)
		verifyHookBehavior(t, "Claude", config, scenario)
	})

	// Test Cursor
	t.Run("Cursor", func(t *testing.T) {
		config := generateCursorConfig(t, scenario.RoleType)
		verifyHookBehavior(t, "Cursor", config, scenario)
	})

	// Test Gemini
	t.Run("Gemini", func(t *testing.T) {
		config := generateGeminiConfig(t, scenario.RoleType)
		verifyHookBehavior(t, "Gemini", config, scenario)
	})
}

// generateClaudeConfig creates a Claude settings file and parses its hooks.
func generateClaudeConfig(t *testing.T, roleType string) RuntimeHookConfig {
	t.Helper()
	dir := t.TempDir()

	var rt claude.RoleType
	if roleType == "autonomous" {
		rt = claude.Autonomous
	} else {
		rt = claude.Interactive
	}

	if err := claude.EnsureSettings(dir, rt); err != nil {
		t.Fatalf("failed to generate Claude settings: %v", err)
	}

	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	return parseClaudeHooks(t, settingsPath)
}

// generateCursorConfig creates a Cursor hooks file and parses its hooks.
func generateCursorConfig(t *testing.T, roleType string) RuntimeHookConfig {
	t.Helper()
	dir := t.TempDir()

	var rt cursor.RoleType
	if roleType == "autonomous" {
		rt = cursor.Autonomous
	} else {
		rt = cursor.Interactive
	}

	if err := cursor.EnsureHooksAt(dir, rt, ".cursor", "hooks.json"); err != nil {
		t.Fatalf("failed to generate Cursor hooks: %v", err)
	}

	hooksPath := filepath.Join(dir, ".cursor", "hooks.json")
	return parseCursorHooks(t, hooksPath)
}

// generateGeminiConfig creates a Gemini settings file and parses its hooks.
func generateGeminiConfig(t *testing.T, roleType string) RuntimeHookConfig {
	t.Helper()
	dir := t.TempDir()

	var rt gemini.RoleType
	if roleType == "autonomous" {
		rt = gemini.Autonomous
	} else {
		rt = gemini.Interactive
	}

	if err := gemini.EnsureSettingsAt(dir, rt, ".gemini", "settings.json"); err != nil {
		t.Fatalf("failed to generate Gemini settings: %v", err)
	}

	settingsPath := filepath.Join(dir, ".gemini", "settings.json")
	return parseGeminiHooks(t, settingsPath)
}

// parseClaudeHooks extracts hooks from a Claude settings.json file.
func parseClaudeHooks(t *testing.T, path string) RuntimeHookConfig {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read Claude settings: %v", err)
	}

	var settings struct {
		Hooks map[string][]struct {
			Matcher string
			Hooks   []struct {
				Type    string
				Command string
			}
		}
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse Claude settings: %v", err)
	}

	config := RuntimeHookConfig{Hooks: make(map[string][]HookEntry)}
	for hookName, matchers := range settings.Hooks {
		for _, m := range matchers {
			for _, h := range m.Hooks {
				config.Hooks[hookName] = append(config.Hooks[hookName], HookEntry{
					Command: h.Command,
					Matcher: m.Matcher,
				})
			}
		}
	}
	return config
}

// parseCursorHooks extracts hooks from a Cursor hooks.json file.
func parseCursorHooks(t *testing.T, path string) RuntimeHookConfig {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read Cursor hooks: %v", err)
	}

	var settings struct {
		Hooks map[string][]struct {
			Command string
			Matcher string
		}
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse Cursor hooks: %v", err)
	}

	config := RuntimeHookConfig{Hooks: make(map[string][]HookEntry)}
	for hookName, entries := range settings.Hooks {
		// Normalize Cursor's camelCase to PascalCase for comparison
		normalizedName := normalizeCursorHookName(hookName)
		for _, e := range entries {
			// Normalize Cursor's Shell() matcher format
			matcher := strings.TrimPrefix(e.Matcher, "Shell(")
			matcher = strings.TrimSuffix(matcher, ")")
			config.Hooks[normalizedName] = append(config.Hooks[normalizedName], HookEntry{
				Command: e.Command,
				Matcher: matcher,
			})
		}
	}
	return config
}

// parseGeminiHooks extracts hooks from a Gemini settings.json file.
func parseGeminiHooks(t *testing.T, path string) RuntimeHookConfig {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read Gemini settings: %v", err)
	}

	var settings struct {
		Hooks map[string][]struct {
			Matcher string
			Hooks   []struct {
				Type    string
				Command string
			}
		}
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse Gemini settings: %v", err)
	}

	config := RuntimeHookConfig{Hooks: make(map[string][]HookEntry)}
	for hookName, matchers := range settings.Hooks {
		// Normalize Gemini's hook names to standard names
		normalizedName := normalizeGeminiHookName(hookName)
		for _, m := range matchers {
			for _, h := range m.Hooks {
				config.Hooks[normalizedName] = append(config.Hooks[normalizedName], HookEntry{
					Command: h.Command,
					Matcher: m.Matcher,
				})
			}
		}
	}
	return config
}

// normalizeCursorHookName converts Cursor's camelCase to standard PascalCase.
// Cursor uses: sessionStart, preToolUse, preCompact, beforeSubmitPrompt, stop
// Standard:    SessionStart, PreToolUse, PreCompact, UserPromptSubmit, Stop
func normalizeCursorHookName(name string) string {
	switch name {
	case "sessionStart":
		return "SessionStart"
	case "preToolUse":
		return "PreToolUse"
	case "preCompact":
		return "PreCompact"
	case "beforeSubmitPrompt":
		return "UserPromptSubmit"
	case "stop":
		return "Stop"
	default:
		return name
	}
}

// normalizeGeminiHookName converts Gemini's hook names to standard names.
// Gemini uses: SessionStart, BeforeTool, PreCompress, BeforeAgent, SessionEnd
// Standard:    SessionStart, PreToolUse, PreCompact, UserPromptSubmit, Stop
func normalizeGeminiHookName(name string) string {
	switch name {
	case "BeforeTool":
		return "PreToolUse"
	case "PreCompress":
		return "PreCompact"
	case "BeforeAgent":
		return "UserPromptSubmit"
	case "SessionEnd":
		return "Stop"
	default:
		return name // SessionStart is already standard
	}
}

// verifyHookBehavior checks that a runtime's hook configuration satisfies all assertions.
func verifyHookBehavior(t *testing.T, runtime string, config RuntimeHookConfig, scenario HookScenario) {
	t.Helper()

	for _, assertion := range scenario.Assertions {
		hooks, exists := config.Hooks[assertion.HookName]
		if !exists && assertion.MustBePresent {
			t.Errorf("%s: missing required hook %q", runtime, assertion.HookName)
			continue
		}
		if !exists {
			continue
		}

		// Find matching hook entry
		var found bool
		for _, hook := range hooks {
			matcherMatches := assertion.Matcher == "" || matcherContains(hook.Matcher, assertion.Matcher)
			if !matcherMatches {
				continue
			}

			// Check command contains all required substrings
			allMatch := true
			for _, substr := range assertion.CommandContains {
				if !strings.Contains(hook.Command, substr) {
					t.Errorf("%s: hook %q (matcher=%q) missing command substring %q in: %s",
						runtime, assertion.HookName, assertion.Matcher, substr, hook.Command)
					allMatch = false
				}
			}
			if allMatch {
				found = true
				break
			}
		}

		if assertion.MustBePresent && !found {
			t.Errorf("%s: hook %q with matcher=%q not found or incomplete",
				runtime, assertion.HookName, assertion.Matcher)
		}
	}
}

// matcherContains checks if a matcher string contains the expected pattern.
// Handles variations like "gh pr create*", "Shell(gh pr create*)", etc.
func matcherContains(matcher, expected string) bool {
	if matcher == "" && expected == "" {
		return true
	}
	// Normalize both by removing wildcards and common prefixes
	normalizedMatcher := strings.TrimSuffix(matcher, "*")
	normalizedExpected := strings.TrimSuffix(expected, "*")
	return strings.Contains(normalizedMatcher, normalizedExpected)
}
