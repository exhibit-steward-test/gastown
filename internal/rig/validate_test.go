package rig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRigName(t *testing.T) {
	// Create temp workspace
	tmpDir := t.TempDir()

	// Create a valid rig structure
	validRig := filepath.Join(tmpDir, "myrig")
	if err := os.MkdirAll(filepath.Join(validRig, "crew"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create another valid rig with different marker
	validRig2 := filepath.Join(tmpDir, "testrig")
	if err := os.MkdirAll(filepath.Join(validRig2, "polecats"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a rig with config.json marker
	validRig3 := filepath.Join(tmpDir, "configrig")
	if err := os.MkdirAll(validRig3, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(validRig3, "config.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create invalid directory (no rig markers)
	invalidDir := filepath.Join(tmpDir, "notarig")
	if err := os.MkdirAll(invalidDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a file (not a directory)
	if err := os.WriteFile(filepath.Join(tmpDir, "afile"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create reserved directories that should be rejected
	for _, reserved := range []string{"mayor", "deacon", ".beads", ".git"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, reserved), 0755); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name      string
		rigName   string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid rig with crew marker",
			rigName:   "myrig",
			wantError: false,
		},
		{
			name:      "valid rig with polecats marker",
			rigName:   "testrig",
			wantError: false,
		},
		{
			name:      "valid rig with config.json marker",
			rigName:   "configrig",
			wantError: false,
		},
		{
			name:      "empty rig name",
			rigName:   "",
			wantError: true,
			errorMsg:  "empty rig name",
		},
		{
			name:      "non-existent rig",
			rigName:   "doesnotexist",
			wantError: true,
			errorMsg:  "not found in workspace",
		},
		{
			name:      "reserved name: hq",
			rigName:   "hq",
			wantError: true,
			errorMsg:  "reserved name",
		},
		{
			name:      "reserved name: mayor",
			rigName:   "mayor",
			wantError: true,
			errorMsg:  "not a rig",
		},
		{
			name:      "reserved name: deacon",
			rigName:   "deacon",
			wantError: true,
			errorMsg:  "not a rig",
		},
		{
			name:      "reserved name: .beads",
			rigName:   ".beads",
			wantError: true,
			errorMsg:  "hidden directory",
		},
		{
			name:      "reserved name: .git",
			rigName:   ".git",
			wantError: true,
			errorMsg:  "hidden directory",
		},
		{
			name:      "reserved name: plugins",
			rigName:   "plugins",
			wantError: true,
			errorMsg:  "not a rig",
		},
		{
			name:      "path traversal: parent directory",
			rigName:   "../somepath",
			wantError: true,
			errorMsg:  "invalid characters",
		},
		{
			name:      "path traversal: embedded parent",
			rigName:   "rig/../mayor",
			wantError: true,
			errorMsg:  "invalid characters",
		},
		{
			name:      "path with forward slash",
			rigName:   "rig/subdir",
			wantError: true,
			errorMsg:  "invalid characters",
		},
		{
			name:      "path with backslash",
			rigName:   "rig\\subdir",
			wantError: true,
			errorMsg:  "invalid characters",
		},
		{
			name:      "hidden directory name",
			rigName:   ".hidden",
			wantError: true,
			errorMsg:  "hidden directory",
		},
		{
			name:      "not a directory (file)",
			rigName:   "afile",
			wantError: true,
			errorMsg:  "not a directory",
		},
		{
			name:      "directory without rig structure",
			rigName:   "notarig",
			wantError: true,
			errorMsg:  "does not have rig structure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRigName(tmpDir, tt.rigName)

			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateRigName(%q) = nil, want error containing %q", tt.rigName, tt.errorMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("ValidateRigName(%q) error = %q, want error containing %q", tt.rigName, err.Error(), tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateRigName(%q) = %v, want nil", tt.rigName, err)
				}
			}
		})
	}
}

func TestListValidRigs(t *testing.T) {
	// Create temp workspace
	tmpDir := t.TempDir()

	// Create valid rigs
	validRigs := []string{"rig1", "rig2", "rig3"}
	for _, rigName := range validRigs {
		rigPath := filepath.Join(tmpDir, rigName)
		if err := os.MkdirAll(filepath.Join(rigPath, "crew"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create invalid directories that should be skipped
	for _, name := range []string{"mayor", "deacon", ".beads", ".git", "plugins"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create a directory without rig structure (should be skipped)
	if err := os.MkdirAll(filepath.Join(tmpDir, "notarig"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a file (should be skipped)
	if err := os.WriteFile(filepath.Join(tmpDir, "afile"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	rigs, err := ListValidRigs(tmpDir)
	if err != nil {
		t.Fatalf("ListValidRigs() error = %v, want nil", err)
	}

	// Check that we got exactly the valid rigs
	if len(rigs) != len(validRigs) {
		t.Errorf("ListValidRigs() returned %d rigs, want %d", len(rigs), len(validRigs))
	}

	// Check that all valid rigs are present
	rigMap := make(map[string]bool)
	for _, rig := range rigs {
		rigMap[rig] = true
	}

	for _, expected := range validRigs {
		if !rigMap[expected] {
			t.Errorf("ListValidRigs() missing expected rig %q", expected)
		}
	}

	// Check that no invalid rigs are present
	invalidNames := []string{"mayor", "deacon", ".beads", ".git", "plugins", "notarig", "afile"}
	for _, invalid := range invalidNames {
		if rigMap[invalid] {
			t.Errorf("ListValidRigs() included invalid rig %q", invalid)
		}
	}
}

func TestValidateRigName_Symlink(t *testing.T) {
	// Create temp workspace
	tmpDir := t.TempDir()

	// Create a valid rig
	realRig := filepath.Join(tmpDir, "realrig")
	if err := os.MkdirAll(filepath.Join(realRig, "crew"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a symlink to the rig
	symlinkRig := filepath.Join(tmpDir, "linkedrig")
	if err := os.Symlink(realRig, symlinkRig); err != nil {
		// Skip test if symlinks are not supported (e.g., Windows without admin)
		if os.IsPermission(err) {
			t.Skip("Symlinks not supported in this environment")
		}
		t.Fatal(err)
	}

	// ValidateRigName should follow the symlink and validate the target
	err := ValidateRigName(tmpDir, "linkedrig")
	if err != nil {
		t.Errorf("ValidateRigName(linkedrig) = %v, want nil (should follow symlink)", err)
	}
}

func TestValidateRigName_ErrorPropagation(t *testing.T) {
	// Create temp workspace
	tmpDir := t.TempDir()

	// Create a rig directory
	rigPath := filepath.Join(tmpDir, "testrig")
	if err := os.MkdirAll(filepath.Join(rigPath, "crew"), 0755); err != nil {
		t.Fatal(err)
	}

	// Make the rig directory unreadable (permission denied)
	if err := os.Chmod(rigPath, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(rigPath, 0755) // Restore permissions for cleanup

	// ValidateRigName should return an error when it cannot access the directory
	err := ValidateRigName(tmpDir, "testrig")
	if err == nil {
		t.Error("ValidateRigName(unreadable rig) = nil, want error")
	}
}
