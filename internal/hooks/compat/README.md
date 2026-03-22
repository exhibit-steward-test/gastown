# Hook Compatibility Test Suite

This package ensures behavioral parity across all Gas Town runtime adapters (Claude Code, Cursor, Gemini, and future runtimes).

## Purpose

The IsAutonomousRole extraction (commit 76ef3fa) proved that logic was silently duplicated across four packages. Without a cross-runtime test matrix, behavioral divergences won't be caught until a guard script works on Claude but silently no-ops on Cursor.

This test suite is the single highest-leverage structural investment for maintaining hook parity.

## What Gets Tested

1. **Role Classification Parity**: All runtimes must classify roles identically (autonomous vs. interactive)
2. **Hook Presence**: Required hooks must exist in all runtime configurations
3. **Hook Commands**: Identical scenarios must execute identical `gt` commands
4. **Tool Matchers**: Pre-tool hooks must guard the same operations across runtimes

## Test Scenarios

### Autonomous Role Hooks
Autonomous roles (polecat, witness, refinery, deacon, boot) require:
- **SessionStart**: `gt prime --hook && gt mail check --inject`
- **PreCompact**: `gt prime --hook`
- **Stop**: `gt costs record`
- **PreToolUse** guards for: `gh pr create`, `git checkout -b`, `git switch -c`

### Interactive Role Hooks
Interactive roles (mayor, crew) require:
- **UserPromptSubmit**: `gt mail check --inject` (Cursor: `beforeSubmitPrompt`)
- **PreCompact**: `gt prime --hook`
- **Stop**: `gt costs record`
- **PreToolUse** guards for: `gh pr create`, `git checkout -b`, `git switch -c`

## Adding New Test Scenarios

When adding new hooks or guard scripts to Gas Town:

1. Define a new `HookScenario` in `compat_test.go`
2. Specify `HookAssertion` entries for each hook point
3. Run `go test ./internal/hooks/compat/...` to verify parity
4. **Do not merge** the new hook until all runtimes pass the compatibility test

Example:
```go
scenario := HookScenario{
    Name:        "Context Budget Guard",
    Description: "All runtimes enforce context budget before expensive operations",
    RoleType:    "autonomous",
    Assertions: []HookAssertion{
        {
            HookName:        "PreToolUse",
            Matcher:         "Bash(*)",
            MustBePresent:   true,
            CommandContains: []string{"gt tap guard context-budget"},
        },
    },
}
```

## Runtime-Specific Normalization

The test suite handles known cross-runtime differences:

- **Cursor** uses camelCase hook names (`sessionStart`) → normalized to PascalCase (`SessionStart`)
- **Cursor** uses `Shell()` matcher prefix → stripped for comparison
- **Claude/Gemini** use nested `hooks` arrays → flattened for comparison

These are **presentation differences only**. Behavioral differences fail the test.

## When to Skip This Suite

If you're adding a hook that is **intentionally runtime-specific** (e.g., exploiting a feature unique to one runtime), document the exception in the hook configuration's inline comment and add a test case verifying the other runtimes explicitly do NOT have that hook.

## Running Tests

```bash
# Run compatibility suite only
go test ./internal/hooks/compat/...

# Verbose output showing all assertions
go test -v ./internal/hooks/compat/...

# Run as part of full test suite
make test
```

## PR Review Checklist

When reviewing PRs that touch `plugins/` or `internal/{claude,cursor,gemini}/`:

- [ ] New hook behavior is implemented for all runtimes OR explicitly documented as runtime-specific
- [ ] Role classification logic routes through `hookutil.IsAutonomousRole`
- [ ] Compatibility suite passes: `go test ./internal/hooks/compat/...`
- [ ] If adding a new hook scenario, corresponding test case added to this suite
