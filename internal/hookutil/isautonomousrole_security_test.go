package hookutil

import (
	"fmt"
	"testing"

	"github.com/steveyegge/gastown/internal/constants"
)

// TestIsAutonomousRole_AllKnownRoles ensures that all documented roles
// in the constants package are tested, and that the function correctly
// classifies each one as autonomous or interactive.
//
// Security relevance: Validates that the function has explicit handling
// for all known roles, reducing the risk that a newly added role defaults
// to an unsafe classification.
func TestIsAutonomousRole_AllKnownRoles(t *testing.T) {
	tests := []struct {
		role       string
		autonomous bool
	}{
		{constants.RolePolecat, true},
		{constants.RoleWitness, true},
		{constants.RoleRefinery, true},
		{constants.RoleDeacon, true},
		{"boot", true}, // Special case: boot is autonomous but not in constants
		{constants.RoleMayor, false},
		{constants.RoleCrew, false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			got := IsAutonomousRole(tt.role)
			if got != tt.autonomous {
				t.Errorf("IsAutonomousRole(%q) = %v, want %v", tt.role, got, tt.autonomous)
			}
		})
	}
}

// TestIsAutonomousRole_MalformedInputs tests the function's behavior with
// malformed or adversarial inputs that could result from env var manipulation
// or injection attempts.
//
// Security relevance: Ensures that invalid role strings fail closed (return false)
// rather than being misclassified as autonomous, which could grant unintended
// automatic privileges.
func TestIsAutonomousRole_MalformedInputs(t *testing.T) {
	malformedInputs := []string{
		"",                         // Empty string
		" ",                        // Whitespace only
		"  polecat  ",              // Leading/trailing whitespace
		"Polecat",                  // Wrong case (should be lowercase)
		"WITNESS",                  // All caps
		"pole cat",                 // Space in middle
		"polecat\n",                // Embedded newline
		"polecat\t",                // Embedded tab
		"polecat\r",                // Embedded carriage return
		"polecat;witness",          // Semicolon delimiter injection
		"polecat,witness",          // Comma delimiter injection
		"polecat|witness",          // Pipe delimiter injection
		"polecat&witness",          // Ampersand delimiter injection
		"polecat||witness",         // Double pipe
		"polecat&&witness",         // Double ampersand
		"polecat:witness",          // Colon delimiter
		"witness/polecat",          // Slash delimiter
		"../witness",               // Path traversal attempt
		"../../polecat",            // Double path traversal
		"witness\x00polecat",       // Null byte injection
		"GT_RIG=witness",           // Env var assignment attempt
		"export GT_RIG=polecat",    // Shell command injection attempt
		"$(echo witness)",          // Shell command substitution
		"`echo polecat`",           // Backtick command substitution
		"witness; rm -rf /",        // Command chaining attempt
		"polecat' OR '1'='1",       // SQL-style injection
		"<script>witness</script>", // XSS-style injection
		"role=polecat",             // Key-value pair format
		"--role=witness",           // Flag-style format
		"-r witness",               // Short flag format
		"unknown_role",             // Arbitrary unrecognized role
		"0",                        // Numeric string
		"true",                     // Boolean string
		"false",                    // Boolean string
		"null",                     // Null string
		"undefined",                // Undefined string
		"witness\nboot",            // Multi-line injection
		"polecat\x0Awitness",       // Hex-encoded newline
	}

	for _, input := range malformedInputs {
		t.Run(fmt.Sprintf("malformed_%q", input), func(t *testing.T) {
			got := IsAutonomousRole(input)
			// All malformed inputs should fail closed and return false
			if got {
				t.Errorf("IsAutonomousRole(%q) = true, want false (fail closed on malformed input)", input)
			}
		})
	}
}

// TestIsAutonomousRole_CaseSensitivity explicitly verifies that the function
// is case-sensitive, which is critical for security: role names must match
// exactly to prevent case-variation bypass attacks.
//
// Security relevance: Prevents an attacker from bypassing role checks by
// varying capitalization (e.g., "Witness" instead of "witness").
func TestIsAutonomousRole_CaseSensitivity(t *testing.T) {
	caseVariations := []string{
		"Polecat",
		"POLECAT",
		"PoLeCAT",
		"Witness",
		"WITNESS",
		"WiTnEsS",
		"Refinery",
		"REFINERY",
		"ReFiNeRy",
		"Deacon",
		"DEACON",
		"DeAcOn",
		"Boot",
		"BOOT",
		"BoOt",
	}

	for _, input := range caseVariations {
		t.Run(input, func(t *testing.T) {
			got := IsAutonomousRole(input)
			// All case variations should fail (function is case-sensitive)
			if got {
				t.Errorf("IsAutonomousRole(%q) = true, want false (case sensitivity required)", input)
			}
		})
	}
}

// TestIsAutonomousRole_UntrustedEnvVarSimulation simulates scenarios where
// GT_ROLE or similar env vars might be set to adversarial values, ensuring
// that the function handles untrusted input safely.
//
// Security relevance: In a multi-agent environment, env vars can be inherited
// or manipulated. This test ensures that even if an untrusted role string is
// passed to IsAutonomousRole, it will not grant autonomous privileges unless
// the string exactly matches a known autonomous role.
func TestIsAutonomousRole_UntrustedEnvVarSimulation(t *testing.T) {
	untrustedEnvVars := []string{
		"GT_ROLE=witness",         // Env var assignment
		"${GT_ROLE}",              // Shell variable expansion
		"$GT_ROLE",                // Shell variable reference
		"{{.Role}}",               // Template interpolation
		"%GT_ROLE%",               // Windows env var syntax
		"~/.gastown/witness",      // Home directory path
		"/tmp/witness",            // Absolute path
		"./witness",               // Relative path
		"witness.exe",             // Executable name
		"witness.sh",              // Script name
		"witness-role",            // Hyphenated variation
		"witness_role",            // Underscored variation
		"role:witness",            // Prefixed format
		"witness@localhost",       // Email-style format
		"witness#123",             // ID-style format
		"witness?autonomous=true", // Query parameter style
		"witness&autonomous",      // URL-encoded style
	}

	for _, input := range untrustedEnvVars {
		t.Run(fmt.Sprintf("untrusted_%q", input), func(t *testing.T) {
			got := IsAutonomousRole(input)
			// All untrusted formats should fail closed
			if got {
				t.Errorf("IsAutonomousRole(%q) = true, want false (untrusted env var format)", input)
			}
		})
	}
}

// TestIsAutonomousRole_BoundaryConditions tests edge cases and boundary
// conditions that might expose parsing or comparison bugs.
//
// Security relevance: Ensures the function handles extreme inputs gracefully
// without panics or unexpected behavior that could be exploited.
func TestIsAutonomousRole_BoundaryConditions(t *testing.T) {
	boundaryInputs := []struct {
		input       string
		description string
	}{
		{"", "empty string"},
		{" ", "single space"},
		{"  ", "double space"},
		{"\t", "single tab"},
		{"\n", "single newline"},
		{"\r\n", "CRLF"},
		{"witness ", "trailing space"},
		{" witness", "leading space"},
		{" witness ", "surrounding spaces"},
		{"wit", "too short (prefix of witness)"},
		{"witnesses", "too long (suffix added)"},
		{"witnes", "almost correct (missing s)"},
		{"witnesa", "typo (a instead of s)"},
		{"pole", "too short (prefix of polecat)"},
		{"polecats", "too long (suffix added)"},
		{"polecat-", "trailing delimiter"},
		{"-polecat", "leading delimiter"},
	}

	for _, tt := range boundaryInputs {
		t.Run(tt.description, func(t *testing.T) {
			got := IsAutonomousRole(tt.input)
			// All boundary cases should fail closed unless they exactly match a known role
			if got {
				t.Errorf("IsAutonomousRole(%q) [%s] = true, want false", tt.input, tt.description)
			}
		})
	}
}

// TestIsAutonomousRole_NoImplicitDefaults verifies that the function does not
// have an implicit default behavior that could grant autonomous status to
// unexpected inputs.
//
// Security relevance: Ensures fail-closed behavior - unknown roles must be
// explicitly rejected, not silently granted autonomous privileges.
func TestIsAutonomousRole_NoImplicitDefaults(t *testing.T) {
	unknownRoles := []string{
		"dog",        // Mentioned in steward brief but not in constants
		"agent",      // Generic term
		"admin",      // Admin-like role
		"root",       // System role
		"sudo",       // Privileged role
		"autonomous", // Meta-role
		"default",    // Default role
		"guest",      // Guest role
		"user",       // User role
		"bot",        // Bot role
		"system",     // System role
		"service",    // Service role
		"daemon",     // Daemon role
		"worker",     // Worker role
		"supervisor", // Supervisor role
	}

	for _, role := range unknownRoles {
		t.Run(role, func(t *testing.T) {
			got := IsAutonomousRole(role)
			if got {
				t.Errorf("IsAutonomousRole(%q) = true, want false (unknown role must fail closed)", role)
			}
		})
	}
}

// TestIsAutonomousRole_UnicodeAndNonASCII tests behavior with unicode and
// non-ASCII characters to ensure the function doesn't have unexpected
// unicode normalization or comparison issues.
//
// Security relevance: Unicode normalization bugs have been used in security
// bypasses. This test ensures the function handles non-ASCII input safely.
func TestIsAutonomousRole_UnicodeAndNonASCII(t *testing.T) {
	unicodeInputs := []string{
		"witneß",        // German sharp s (looks like ss)
		"witnеss",       // Cyrillic 'е' instead of Latin 'e' (homoglyph attack)
		"ｗｉｔｎｅｓｓ",       // Full-width characters
		"witness\u200B", // Zero-width space
		"wit\u200Bness", // Zero-width space in middle
		"witness\u202E", // Right-to-left override
		"pole\u0301cat", // Combining acute accent
		"🦉witness",      // Emoji prefix (witness emoji from constants)
		"witness🦉",      // Emoji suffix
		"見證者",           // Chinese characters (means "witness")
		"شاهد",          // Arabic (means "witness")
		"witness\u0000", // Null byte
		"witness\uFEFF", // Zero-width no-break space (BOM)
	}

	for _, input := range unicodeInputs {
		t.Run(fmt.Sprintf("unicode_%q", input), func(t *testing.T) {
			got := IsAutonomousRole(input)
			// All unicode variations should fail (exact ASCII match required)
			if got {
				t.Errorf("IsAutonomousRole(%q) = true, want false (non-ASCII variations must fail)", input)
			}
		})
	}
}
