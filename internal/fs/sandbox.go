package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	ErrAccessDenied    = errors.New("access denied: path outside root directory")
	ErrInvalidPath     = errors.New("invalid path: contains illegal characters or sequence")
	ErrCannotRenameRoot = errors.New("cannot rename root directory")
	ErrTargetExists    = errors.New("target already exists")
	ErrNotADirectory   = errors.New("target is not a directory")
)

// Windows reserved device names
var reservedWindowsNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true,
	"COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true,
	"LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

var invalidNameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)

// SanitizeSubPath cleans and validates a relative path
func SanitizeSubPath(subPath string) (string, error) {
	// Reject null bytes
	if strings.ContainsRune(subPath, 0) {
		return "", ErrInvalidPath
	}

	// Raw traversal check on input
	rawSlash := filepath.ToSlash(subPath)
	if strings.HasPrefix(rawSlash, "..") || strings.Contains(rawSlash, "/..") || strings.Contains(rawSlash, "../") {
		return "", ErrAccessDenied
	}

	// Normalize slashes to forward slashes for uniform handling
	cleaned := filepath.ToSlash(filepath.Clean(subPath))
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = strings.TrimPrefix(cleaned, "./")

	if cleaned == "." || cleaned == "" {
		return "", nil
	}

	// Traversal check after clean
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, "/../") || strings.HasSuffix(cleaned, "/..") {
		return "", ErrAccessDenied
	}

	// Check each path segment for Windows reserved device names or stream colons
	parts := strings.Split(cleaned, "/")
	for _, part := range parts {
		upper := strings.ToUpper(part)
		// Check for Alternate Data Streams (ADS)
		if strings.Contains(upper, ":") {
			return "", ErrInvalidPath
		}
		// Strip extension for reserved name check (e.g. NUL.txt is also reserved on Windows)
		baseName := strings.ToUpper(strings.TrimSuffix(part, filepath.Ext(part)))
		if reservedWindowsNames[upper] || reservedWindowsNames[baseName] {
			return "", ErrInvalidPath
		}
	}

	return cleaned, nil
}

// ResolveSandboxedPath resolves a subpath strictly within rootDir
func ResolveSandboxedPath(rootDir, subPath string) (string, error) {
	cleanSub, err := SanitizeSubPath(subPath)
	if err != nil {
		return "", err
	}

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute root path: %w", err)
	}

	// Ensure root folder exists
	_ = os.MkdirAll(absRoot, 0755)

	// Combine root and cleaned subpath
	targetPath := filepath.Join(absRoot, filepath.FromSlash(cleanSub))
	targetPath = filepath.Clean(targetPath)

	// Ensure targetPath starts with absRoot
	rel, err := filepath.Rel(absRoot, targetPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrAccessDenied
	}

	// If the path exists on disk, also check symlinks
	if targetStat, err := os.Lstat(targetPath); err == nil {
		if targetStat.Mode()&os.ModeSymlink != 0 {
			evalTarget, err := filepath.EvalSymlinks(targetPath)
			if err != nil {
				return "", ErrAccessDenied
			}
			evalRoot, err := filepath.EvalSymlinks(absRoot)
			if err != nil {
				evalRoot = absRoot
			}
			relEval, err := filepath.Rel(evalRoot, evalTarget)
			if err != nil || relEval == ".." || strings.HasPrefix(relEval, ".."+string(filepath.Separator)) {
				return "", ErrAccessDenied
			}
		}
	}

	return targetPath, nil
}

// ValidateNewFolderName checks if a folder name is valid (single folder name, no path separators or reserved names)
func ValidateNewFolderName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return errors.New("folder name cannot be empty, '.' or '..'")
	}
	if len(name) > 255 {
		return errors.New("folder name exceeds maximum length of 255 characters")
	}
	if invalidNameChars.MatchString(name) {
		return errors.New("folder name contains invalid characters (< > : \" / \\ | ? *)")
	}
	upper := strings.ToUpper(name)
	base := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
	if reservedWindowsNames[upper] || reservedWindowsNames[base] {
		return errors.New("folder name is a reserved Windows system name")
	}
	return nil
}

// SafeRenameFolder renames a folder inside rootDir safely
func SafeRenameFolder(rootDir, relPath, newName string) (string, error) {
	cleanRel, err := SanitizeSubPath(relPath)
	if err != nil {
		return "", err
	}
	if cleanRel == "" {
		return "", ErrCannotRenameRoot
	}

	if err := ValidateNewFolderName(newName); err != nil {
		return "", err
	}

	oldFullPath, err := ResolveSandboxedPath(rootDir, cleanRel)
	if err != nil {
		return "", err
	}

	stat, err := os.Stat(oldFullPath)
	if err != nil {
		return "", fmt.Errorf("source folder not found: %w", err)
	}
	if !stat.IsDir() {
		return "", ErrNotADirectory
	}

	// Compute new full path in the same parent directory
	parentDir := filepath.Dir(oldFullPath)
	newFullPath := filepath.Join(parentDir, newName)

	// Validate new full path is still strictly inside rootDir
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, newFullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrAccessDenied
	}

	// Check if target already exists
	if _, err := os.Stat(newFullPath); !os.IsNotExist(err) {
		return "", ErrTargetExists
	}

	// Perform rename
	if err := os.Rename(oldFullPath, newFullPath); err != nil {
		return "", fmt.Errorf("failed to rename folder: %w", err)
	}

	// Return new relative path with forward slashes
	newRel, _ := filepath.Rel(absRoot, newFullPath)
	return filepath.ToSlash(newRel), nil
}
