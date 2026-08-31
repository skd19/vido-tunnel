package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/skd19/vido-tunnel/internal/auth"
	"github.com/skd19/vido-tunnel/internal/config"
	vfs "github.com/skd19/vido-tunnel/internal/fs"
	"github.com/skd19/vido-tunnel/internal/process"
	"github.com/skd19/vido-tunnel/web"
)

type Server struct {
	cfg         *config.Config
	authMgr     *auth.Manager
	rateLimiter *auth.RateLimiter
	procMgr     *process.Manager
	tunnelMgr   *process.TunnelManager
	pages       map[string]*template.Template
	onExit      func(stopVidoveo, stopTunnel, shutdownPC bool)
}

func NewServer(cfg *config.Config, authMgr *auth.Manager, rateLimiter *auth.RateLimiter, procMgr *process.Manager, tunnelMgr *process.TunnelManager) (*Server, error) {
	// Parse templates from embedded filesystem
	tmplFS, err := fs.Sub(web.Content, "templates")
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded templates: %w", err)
	}

	pages := make(map[string]*template.Template)
	pageNames := []string{"login.html", "explorer.html", "control.html"}
	for _, p := range pageNames {
		tmpl, err := template.ParseFS(tmplFS, "base.html", p)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", p, err)
		}
		pages[p] = tmpl
	}

	return &Server{
		cfg:         cfg,
		authMgr:     authMgr,
		rateLimiter: rateLimiter,
		procMgr:     procMgr,
		tunnelMgr:   tunnelMgr,
		pages:       pages,
	}, nil
}

// SetOnExit sets the callback invoked when exit is requested via HTTP Control Panel
func (s *Server) SetOnExit(fn func(stopVidoveo, stopTunnel, shutdownPC bool)) {
	s.onExit = fn
}

// HandleLogin renders and processes the secret key authentication form
func (s *Server) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if s.authMgr.IsAuthenticated(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	ip := auth.ExtractIP(r)

	if r.Method == http.MethodGet {
		s.renderTemplate(w, "login.html", map[string]interface{}{
			"Authenticated": false,
			"Error":         "",
		})
		return
	}

	if r.Method == http.MethodPost {
		allowed, waitDuration := s.rateLimiter.IsAllowed(ip)
		if !allowed {
			s.renderTemplate(w, "login.html", map[string]interface{}{
				"Authenticated": false,
				"Error":         fmt.Sprintf("Too many failed attempts. Please wait %d seconds.", int(waitDuration.Seconds())),
			})
			return
		}

		key := r.FormValue("key")
		if s.authMgr.ValidateKey(key) {
			s.rateLimiter.RecordSuccess(ip)
			s.authMgr.SetSessionCookie(w, r)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		locked, lockWait := s.rateLimiter.RecordFailure(ip)
		errMsg := "Invalid secret key."
		if locked {
			errMsg = fmt.Sprintf("Too many failed attempts. Locked out for %d seconds.", int(lockWait.Seconds()))
		}

		s.renderTemplate(w, "login.html", map[string]interface{}{
			"Authenticated": false,
			"Error":         errMsg,
		})
	}
}

// HandleLogout terminates the session
func (s *Server) HandleLogout(w http.ResponseWriter, r *http.Request) {
	s.authMgr.ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// HandleBrowse renders the directory explorer (supports POST path and GET root)
func (s *Server) HandleBrowse(w http.ResponseWriter, r *http.Request) {
	relPath := ""
	if r.Method == http.MethodPost {
		relPath = r.FormValue("path")
	}

	dirView, err := vfs.ListDirectory(s.cfg.RootDir, relPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error accessing folder: %v", err), http.StatusBadRequest)
		return
	}

	s.renderTemplate(w, "explorer.html", map[string]interface{}{
		"Authenticated": true,
		"ActiveNav":     "explorer",
		"RootDir":       s.cfg.RootDir,
		"View":          dirView,
	})
}

// HandleDownload streams single files with support for Range requests (video streaming/seeking)
func (s *Server) HandleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed. Downloads must use POST.", http.StatusMethodNotAllowed)
		return
	}

	relPath := r.FormValue("path")
	fullPath, err := vfs.ResolveSandboxedPath(s.cfg.RootDir, relPath)
	if err != nil {
		http.Error(w, "Access Denied", http.StatusForbidden)
		return
	}

	file, err := os.Open(fullPath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil || stat.IsDir() {
		http.Error(w, "Cannot download directories as single file", http.StatusBadRequest)
		return
	}

	filename := filepath.Base(fullPath)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	http.ServeContent(w, r, filename, stat.ModTime(), file)
}

// HandleDownloadZip streams an entire directory as a ZIP archive on-the-fly
func (s *Server) HandleDownloadZip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed. ZIP downloads must use POST.", http.StatusMethodNotAllowed)
		return
	}

	relPath := r.FormValue("path")
	fullPath, err := vfs.ResolveSandboxedPath(s.cfg.RootDir, relPath)
	if err != nil {
		http.Error(w, "Access Denied", http.StatusForbidden)
		return
	}

	stat, err := os.Stat(fullPath)
	if err != nil || !stat.IsDir() {
		http.Error(w, "Target is not a valid directory", http.StatusBadRequest)
		return
	}

	zipName := vfs.GetZipArchiveFilename(relPath)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipName))

	if err := vfs.StreamFolderZip(w, s.cfg.RootDir, relPath); err != nil {
		log.Printf("Error streaming zip: %v", err)
	}
}

// HandleRenameFolder renames a folder securely
func (s *Server) HandleRenameFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	relPath := r.FormValue("path")
	newName := r.FormValue("new_name")

	w.Header().Set("Content-Type", "application/json")

	newRel, err := vfs.SafeRenameFolder(s.cfg.RootDir, relPath, newName)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "Folder renamed successfully",
		"new_path": newRel,
	})
}

// HandleControlPanel renders the system control panel page
func (s *Server) HandleControlPanel(w http.ResponseWriter, r *http.Request) {
	status := s.procMgr.GetStatus()
	tunnelStatus := s.tunnelMgr.GetStatus()

	s.renderTemplate(w, "control.html", map[string]interface{}{
		"Authenticated": true,
		"ActiveNav":     "control",
		"RootDir":       s.cfg.RootDir,
		"Status":        status,
		"Tunnel":        tunnelStatus,
	})
}

// HandleControlStatus returns JSON status of Vidoveo application, port 7788, and Cloudflare tunnel
func (s *Server) HandleControlStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	status := s.procMgr.GetStatus()
	tunnelStatus := s.tunnelMgr.GetStatus()

	response := map[string]interface{}{
		"name":         status.Name,
		"exec_path":    status.ExecPath,
		"path_exists":  status.PathExists,
		"running":      status.Running,
		"pid":          status.PID,
		"port":         status.Port,
		"port_open":    status.PortOpen,
		"last_checked": status.LastChecked,
		"message":      status.Message,
		"app":          status,
		"tunnel":       tunnelStatus,
	}

	_ = json.NewEncoder(w).Encode(response)
}

// HandleControlStart initiates starting Vidoveo.exe
func (s *Server) HandleControlStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err := s.procMgr.Start()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Application started successfully",
	})
}

// HandleControlStop terminates Vidoveo.exe
func (s *Server) HandleControlStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err := s.procMgr.Stop()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Application stopped successfully",
	})
}

// HandleControlTunnelStart launches the Cloudflare tunnel
func (s *Server) HandleControlTunnelStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err := s.tunnelMgr.Start()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Cloudflare tunnel '%s' started successfully", s.cfg.TunnelName),
	})
}

// HandleControlTunnelStop terminates the Cloudflare tunnel
func (s *Server) HandleControlTunnelStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err := s.tunnelMgr.Stop()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Cloudflare tunnel '%s' stopped successfully", s.cfg.TunnelName),
	})
}

// HandleControlTunnelRestart stops and starts the Cloudflare tunnel fresh
func (s *Server) HandleControlTunnelRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err := s.tunnelMgr.Restart()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Cloudflare tunnel '%s' restarted successfully", s.cfg.TunnelName),
	})
}

// HandleControlAppExit closes Vido Tunnel, terminates child processes, and optionally shuts down the PC
func (s *Server) HandleControlAppExit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	stopVidoveo := r.FormValue("stop_vidoveo") != "false" && r.FormValue("stop_vidoveo") != "0"
	stopTunnel := r.FormValue("stop_tunnel") != "false" && r.FormValue("stop_tunnel") != "0"
	shutdownPC := r.FormValue("shutdown_pc") == "true" || r.FormValue("shutdown_pc") == "1"

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"message":      "Server shutdown initiated. Closing application...",
		"stop_vidoveo": stopVidoveo,
		"stop_tunnel":  stopTunnel,
		"shutdown_pc":  shutdownPC,
	})

	if s.onExit != nil {
		go func() {
			time.Sleep(500 * time.Millisecond)
			s.onExit(stopVidoveo, stopTunnel, shutdownPC)
		}()
	}
}

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, exists := s.pages[name]
	if !exists {
		log.Printf("Template %s not found in registry", name)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err := t.ExecuteTemplate(w, "base.html", data)
	if err != nil {
		log.Printf("Template execution error for %s: %v", name, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
