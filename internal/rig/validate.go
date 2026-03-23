package rig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrInvalidRigName is returned when a rig name is invalid or unrecognized.
var ErrInvalidRigName = fmt.Errorf("invalid rig name")

// ValidateRigName validates that a rig name exists in the workspace and
// is a legitimate rig directory. This is a fail-closed security check:
// it rejects reserved names, non-existent directories, and paths that
// don't match the rig directory structure.
//
// This function is the authoritative validator for GT_RIG environment
// variable values. It ensures agent identity cannot be spoofed by
// setting GT_RIG to an arbitrary or malicious value.
//
// Returns nil if the rig name is valid, or ErrInvalidRigName if not.
func ValidateRigName(townRoot, rigName string) error {
	// Reject empty rig name
	if rigName == "" {
		return fmt.Errorf("%w: empty rig name", ErrInvalidRigName)
	}

	// Reject path traversal attempts (e.g., "../mayor", "rig/../mayor")
	if strings.Contains(rigName, "..") || strings.Contains(rigName, "/") || strings.Contains(rigName, "\\") {
		return fmt.Errorf("%w: %q contains invalid characters", ErrInvalidRigName, rigName)
	}

	// Reject hidden directories (starting with .)
	if strings.HasPrefix(rigName, ".") {
		return fmt.Errorf("%w: %q is a hidden directory", ErrInvalidRigName, rigName)
	}

	// Reject reserved names that collide with town-level infrastructure
	for _, reserved := range reservedRigNames {
		if rigName == reserved {
			return fmt.Errorf("%w: %q is a reserved name", ErrInvalidRigName, rigName)
		}
	}

	// Reject reserved directory names that are never rigs
	switch rigName {
	case "mayor", "deacon", "plugins":
		return fmt.Errorf("%w: %q is not a rig", ErrInvalidRigName, rigName)
	}

	// Verify the rig directory exists
	rigPath := filepath.Join(townRoot, rigName)
	info, err := os.Stat(rigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: rig %q not found in workspace", ErrInvalidRigName, rigName)
		}
		return fmt.Errorf("%w: cannot access rig %q: %v", ErrInvalidRigName, rigName, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%w: %q is not a directory", ErrInvalidRigName, rigName)
	}

	// Verify this looks like a rig by checking for rig structure markers.
	// A valid rig must have at least one of: crew/, polecats/, witness/, refinery/, or config.json
	markers := []string{
		filepath.Join(rigPath, "crew"),
		filepath.Join(rigPath, "polecats"),
		filepath.Join(rigPath, "witness"),
		filepath.Join(rigPath, "refinery"),
		filepath.Join(rigPath, "config.json"),
	}

	hasMarker := false
	for _, marker := range markers {
		if _, err := os.Stat(marker); err == nil {
			hasMarker = true
			break
		}
	}

	if !hasMarker {
		return fmt.Errorf("%w: %q does not have rig structure (missing crew/, polecats/, witness/, refinery/, or config.json)", ErrInvalidRigName, rigName)
	}

	return nil
}

// ListValidRigs returns all valid rig names in the workspace.
// This provides the "canonical registry" of known rigs that ValidateRigName
// checks against. Rigs that fail validation are skipped.
func ListValidRigs(townRoot string) ([]string, error) {
	var rigs []string

	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return nil, fmt.Errorf("reading workspace: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		rigName := entry.Name()

		// Use ValidateRigName to ensure consistency with the validation logic
		if ValidateRigName(townRoot, rigName) == nil {
			rigs = append(rigs, rigName)
		}
	}

	return rigs, nil
}
