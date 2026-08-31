package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type FileItem struct {
	Name             string    `json:"name"`
	RelPath          string    `json:"rel_path"`
	IsDir            bool      `json:"is_dir"`
	Size             int64     `json:"size"`
	FormattedSize    string    `json:"formatted_size"`
	ModTime          time.Time `json:"mod_time"`
	FormattedModTime string    `json:"formatted_mod_time"`
	Ext              string    `json:"ext"`
	Category         string    `json:"category"`
	IconClass        string    `json:"icon_class"`
}

type Breadcrumb struct {
	Name     string `json:"name"`
	RelPath  string `json:"rel_path"`
	IsActive bool   `json:"is_active"`
}

type DirectoryView struct {
	CurrentRelPath     string       `json:"current_rel_path"`
	ParentRelPath      string       `json:"parent_rel_path"`
	HasParent          bool         `json:"has_parent"`
	Breadcrumbs        []Breadcrumb `json:"breadcrumbs"`
	Items              []FileItem   `json:"items"`
	TotalDirs          int          `json:"total_dirs"`
	TotalFiles         int          `json:"total_files"`
	TotalSize          int64        `json:"total_size"`
	FormattedTotalSize string       `json:"formatted_total_size"`
}

// ListDirectory reads and formats the directory contents inside rootDir
func ListDirectory(rootDir, subPath string) (*DirectoryView, error) {
	cleanRel, err := SanitizeSubPath(subPath)
	if err != nil {
		return nil, err
	}

	fullPath, err := ResolveSandboxedPath(rootDir, cleanRel)
	if err != nil {
		return nil, err
	}

	// Ensure root folder exists if browsing root
	if cleanRel == "" {
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			_ = os.MkdirAll(fullPath, 0755)
		}
	}

	dirEntries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var items []FileItem
	var totalDirs, totalFiles int
	var totalSize int64

	for _, entry := range dirEntries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		name := entry.Name()
		itemRel := name
		if cleanRel != "" {
			itemRel = cleanRel + "/" + name
		}

		isDir := entry.IsDir()
		var size int64
		var ext string
		var cat, icon string

		if isDir {
			totalDirs++
			cat = "folder"
			icon = "bi-folder-fill text-warning"
		} else {
			totalFiles++
			size = info.Size()
			totalSize += size
			ext = strings.ToLower(filepath.Ext(name))
			cat, icon = categorizeFile(ext)
		}

		items = append(items, FileItem{
			Name:             name,
			RelPath:          itemRel,
			IsDir:            isDir,
			Size:             size,
			FormattedSize:    FormatBytes(size, isDir),
			ModTime:          info.ModTime(),
			FormattedModTime: info.ModTime().Format("2006-01-02 15:04"),
			Ext:              ext,
			Category:         cat,
			IconClass:        icon,
		})
	}

	// Sort items: folders first, then files alphabetically (case-insensitive)
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	breadcrumbs := buildBreadcrumbs(cleanRel)

	var parentRel string
	hasParent := cleanRel != ""
	if hasParent {
		parentRel = filepath.ToSlash(filepath.Dir(cleanRel))
		if parentRel == "." {
			parentRel = ""
		}
	}

	return &DirectoryView{
		CurrentRelPath:     cleanRel,
		ParentRelPath:      parentRel,
		HasParent:          hasParent,
		Breadcrumbs:        breadcrumbs,
		Items:              items,
		TotalDirs:          totalDirs,
		TotalFiles:         totalFiles,
		TotalSize:          totalSize,
		FormattedTotalSize: FormatBytes(totalSize, false),
	}, nil
}

func buildBreadcrumbs(cleanRel string) []Breadcrumb {
	crumbs := []Breadcrumb{
		{Name: "Root", RelPath: "", IsActive: cleanRel == ""},
	}

	if cleanRel == "" {
		return crumbs
	}

	parts := strings.Split(cleanRel, "/")
	var accum string
	for i, part := range parts {
		if accum == "" {
			accum = part
		} else {
			accum += "/" + part
		}
		crumbs = append(crumbs, Breadcrumb{
			Name:     part,
			RelPath:  accum,
			IsActive: i == len(parts)-1,
		})
	}
	return crumbs
}

func categorizeFile(ext string) (string, string) {
	switch ext {
	case ".mp4", ".mkv", ".webm", ".avi", ".mov", ".flv", ".wmv", ".m4v", ".ts":
		return "video", "bi-film text-danger"
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".m4a", ".wma":
		return "audio", "bi-music-note-beamed text-info"
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".bmp", ".ico":
		return "image", "bi-image text-success"
	case ".zip", ".tar", ".gz", ".7z", ".rar", ".bz2", ".xz":
		return "archive", "bi-file-earmark-zip text-primary"
	case ".pdf", ".txt", ".md", ".json", ".xml", ".csv", ".docx", ".xlsx", ".pptx", ".log":
		return "doc", "bi-file-earmark-text text-secondary"
	default:
		return "other", "bi-file-earmark text-muted"
	}
}

// FormatBytes formats byte sizes into human readable strings
func FormatBytes(b int64, isDir bool) string {
	if isDir {
		return "--"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
