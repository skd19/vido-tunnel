package fs

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// StreamFolderZip compresses a directory and streams the ZIP archive directly to w
func StreamFolderZip(w io.Writer, rootDir, relPath string) error {
	fullPath, err := ResolveSandboxedPath(rootDir, relPath)
	if err != nil {
		return err
	}

	stat, err := os.Stat(fullPath)
	if err != nil {
		return fmt.Errorf("source not found: %w", err)
	}
	if !stat.IsDir() {
		return ErrNotADirectory
	}

	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	// Walk the target folder
	err = filepath.Walk(fullPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			// Skip unreadable files rather than aborting the entire zip
			return nil
		}

		// Compute path relative to the target folder being zipped
		relToTarget, err := filepath.Rel(fullPath, path)
		if err != nil || relToTarget == "." {
			return nil
		}

		// Ensure forward slashes in zip header paths
		zipEntryPath := filepath.ToSlash(relToTarget)

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return nil
		}

		header.Name = zipEntryPath

		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return nil
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer file.Close()

			_, _ = io.Copy(writer, file)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error building zip stream: %w", err)
	}

	return zipWriter.Flush()
}

// GetZipArchiveFilename generates a clean safe filename for the downloaded zip
func GetZipArchiveFilename(relPath string) string {
	clean := strings.Trim(filepath.ToSlash(relPath), "/")
	if clean == "" || clean == "." {
		return "vidoveo_root_archive.zip"
	}
	base := filepath.Base(clean)
	return fmt.Sprintf("%s.zip", base)
}
