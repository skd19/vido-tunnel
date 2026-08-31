package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skd19/vido-tunnel/internal/auth"
	"github.com/skd19/vido-tunnel/internal/config"
	"github.com/skd19/vido-tunnel/internal/process"
)

func setupTestServer(t *testing.T) (*Server, *config.Config, *auth.Manager, string) {
	tempDir, err := os.MkdirTemp("", "vido_handlers_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create test subfolder and file
	testSub := filepath.Join(tempDir, "sample_folder")
	_ = os.Mkdir(testSub, 0755)
	_ = os.WriteFile(filepath.Join(testSub, "video.mp4"), []byte("dummy video content"), 0644)

	cfg := &config.Config{
		RootDir:       tempDir,
		SecretKey:     "test-secret-123",
		Port:          "8080",
		VidoveoPath:   "C:\\Vidoveo\\Vidoveo.exe",
		VidoveoPort:   7788,
		SessionSecret: []byte("12345678901234567890123456789012"),
	}

	authMgr := auth.NewManager(cfg.SecretKey, cfg.SessionSecret)
	rateLimiter := auth.NewRateLimiter(5, 5*time.Minute)
	procMgr := process.NewManager(cfg.VidoveoPath, cfg.VidoveoPort)
	tunnelMgr := process.NewTunnelManager(cfg.TunnelName, cfg.CloudflaredPath)

	srv, err := NewServer(cfg, authMgr, rateLimiter, procMgr, tunnelMgr)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	return srv, cfg, authMgr, tempDir
}

func TestHandleLogin_AuthFlow(t *testing.T) {
	srv, _, authMgr, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// GET login
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rr := httptest.NewRecorder()
	srv.HandleLogin(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET /login returned %d, want %d", rr.Code, http.StatusOK)
	}

	// POST wrong key
	form := url.Values{}
	form.Set("key", "wrong-key")
	req = httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	srv.HandleLogin(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Invalid secret key") {
		t.Errorf("POST /login with wrong key did not display error")
	}

	// POST correct key
	form.Set("key", "test-secret-123")
	req = httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	srv.HandleLogin(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("POST /login with correct key returned %d, want %d", rr.Code, http.StatusSeeOther)
	}

	// Check cookie
	cookies := rr.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == auth.CookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || !authMgr.ValidateSessionToken(sessionCookie.Value) {
		t.Errorf("Valid session cookie was not set")
	}
}

func TestHandleBrowse_Protected(t *testing.T) {
	srv, _, authMgr, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	handler := RequireAuth(authMgr, srv.HandleBrowse)

	// Unauthenticated request
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("Unauthenticated GET / returned %d, want %d redirect", rr.Code, http.StatusSeeOther)
	}

	// Authenticated request
	token := authMgr.CreateSessionToken()
	req = httptest.NewRequest(http.MethodPost, "/browse", strings.NewReader("path=sample_folder"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Authenticated POST /browse returned %d, want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), "video.mp4") {
		t.Errorf("Browse response missing expected file 'video.mp4'")
	}
}

func TestHandleRename_POST(t *testing.T) {
	srv, _, authMgr, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	handler := RequireAuth(authMgr, srv.HandleRenameFolder)
	token := authMgr.CreateSessionToken()

	form := url.Values{}
	form.Set("path", "sample_folder")
	form.Set("new_name", "renamed_folder")

	req := httptest.NewRequest(http.MethodPost, "/rename", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST /rename returned %d, want %d", rr.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil || resp["success"] != true {
		t.Errorf("Rename response invalid: %v", resp)
	}

	// Verify disk
	if _, err := os.Stat(filepath.Join(tempDir, "renamed_folder")); os.IsNotExist(err) {
		t.Errorf("Folder was not renamed on disk")
	}
}

func TestHandleDownload_File(t *testing.T) {
	srv, _, authMgr, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	handler := RequireAuth(authMgr, srv.HandleDownload)
	token := authMgr.CreateSessionToken()

	req := httptest.NewRequest(http.MethodPost, "/download", strings.NewReader("path=sample_folder/video.mp4"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST /download returned %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != "dummy video content" {
		t.Errorf("Downloaded content mismatch: got %q", rr.Body.String())
	}
}

func TestHandleDownloadZip_Folder(t *testing.T) {
	srv, _, authMgr, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	handler := RequireAuth(authMgr, srv.HandleDownloadZip)
	token := authMgr.CreateSessionToken()

	req := httptest.NewRequest(http.MethodPost, "/download/zip", strings.NewReader("path=sample_folder"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST /download/zip returned %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Header().Get("Content-Type") != "application/zip" {
		t.Errorf("ZIP header mismatch: got %q", rr.Header().Get("Content-Type"))
	}
	if rr.Body.Len() == 0 {
		t.Errorf("ZIP stream is empty")
	}
}

func TestHandleControlStatus(t *testing.T) {
	srv, _, authMgr, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	handler := RequireAuth(authMgr, srv.HandleControlStatus)
	token := authMgr.CreateSessionToken()

	req := httptest.NewRequest(http.MethodPost, "/control/status", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST /control/status returned %d, want %d", rr.Code, http.StatusOK)
	}

	var status process.ProcessStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Errorf("Failed to decode status JSON: %v", err)
	}
	if status.Port != 7788 {
		t.Errorf("Status port = %d, want 7788", status.Port)
	}
}

func TestHandleControlAppExit(t *testing.T) {
	srv, _, authMgr, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	exitCalled := false
	shutdownPCCalled := false
	srv.SetOnExit(func(shutdownPC bool) {
		exitCalled = true
		shutdownPCCalled = shutdownPC
	})

	handler := RequireAuth(authMgr, srv.HandleControlAppExit)
	token := authMgr.CreateSessionToken()

	form := url.Values{}
	form.Set("shutdown_pc", "true")

	req := httptest.NewRequest(http.MethodPost, "/control/app/exit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST /control/app/exit returned %d, want %d", rr.Code, http.StatusOK)
	}

	time.Sleep(600 * time.Millisecond)
	if !exitCalled {
		t.Errorf("onExit callback was not triggered")
	}
	if !shutdownPCCalled {
		t.Errorf("shutdownPC flag was not passed as true")
	}
}

