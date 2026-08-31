package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeSubPath(t *testing.T) {
	tests := []struct {
		input   string
		expect  string
		wantErr bool
	}{
		{"", "", false},
		{".", "", false},
		{"/", "", false},
		{"subfolder", "subfolder", false},
		{"subfolder/nested", "subfolder/nested", false},
		{"subfolder\\nested", "subfolder/nested", false},
		{"../escape", "", true},
		{"sub/../../escape", "", true},
		{"/../escape", "", true},
		{"sub/\x00null", "", true},
		{"sub/CON", "", true},
		{"sub/NUL.txt", "", true},
		{"sub/file.txt:stream", "", true},
	}

	for _, tt := range tests {
		got, err := SanitizeSubPath(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("SanitizeSubPath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if err == nil && got != tt.expect {
			t.Errorf("SanitizeSubPath(%q) = %q, want %q", tt.input, got, tt.expect)
		}
	}
}

func TestResolveSandboxedPath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vido_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	subDir := filepath.Join(tempDir, "allowed")
	_ = os.Mkdir(subDir, 0755)

	// Valid path
	resolved, err := ResolveSandboxedPath(tempDir, "allowed")
	if err != nil {
		t.Errorf("ResolveSandboxedPath valid failed: %v", err)
	}
	if resolved != subDir {
		t.Errorf("ResolveSandboxedPath = %s, want %s", resolved, subDir)
	}

	// Traversal attempt
	_, err = ResolveSandboxedPath(tempDir, "../outside")
	if err == nil {
		t.Errorf("ResolveSandboxedPath expected error on traversal, got nil")
	}
}

func TestSafeRenameFolder(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vido_test_rename_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	subFolder := filepath.Join(tempDir, "old_name")
	if err := os.Mkdir(subFolder, 0755); err != nil {
		t.Fatalf("Failed to create test folder: %v", err)
	}

	// Rename root should fail
	_, err = SafeRenameFolder(tempDir, "", "new_root")
	if err == nil {
		t.Errorf("Expected error renaming root folder, got nil")
	}

	// Rename with illegal characters should fail
	_, err = SafeRenameFolder(tempDir, "old_name", "new/name")
	if err == nil {
		t.Errorf("Expected error renaming with slashes, got nil")
	}

	// Valid rename
	newRel, err := SafeRenameFolder(tempDir, "old_name", "new_name")
	if err != nil {
		t.Fatalf("SafeRenameFolder failed: %v", err)
	}
	if newRel != "new_name" {
		t.Errorf("SafeRenameFolder returned %s, want new_name", newRel)
	}

	// Verify on disk
	if _, err := os.Stat(filepath.Join(tempDir, "new_name")); os.IsNotExist(err) {
		t.Errorf("New folder does not exist on disk")
	}
	if _, err := os.Stat(filepath.Join(tempDir, "old_name")); !os.IsNotExist(err) {
		t.Errorf("Old folder still exists on disk")
	}
}
