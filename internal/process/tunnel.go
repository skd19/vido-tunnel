package process

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type TunnelStatus struct {
	TunnelName string `json:"tunnel_name"`
	BinaryPath string `json:"binary_path"`
	Installed  bool   `json:"installed"`
	Running    bool   `json:"running"`
	PID        int    `json:"pid"`
	Message    string `json:"message"`
}

type TunnelManager struct {
	mu         sync.Mutex
	tunnelName string
	customPath string
	lastPID    int
}

// NewTunnelManager creates a new Cloudflare tunnel supervisor
func NewTunnelManager(tunnelName string, customPath string) *TunnelManager {
	if tunnelName == "" {
		tunnelName = "vidoveo"
	}
	return &TunnelManager{
		tunnelName: tunnelName,
		customPath: customPath,
	}
}

// FindCloudflared locates the cloudflared executable on the system
func (tm *TunnelManager) FindCloudflared() string {
	if tm.customPath != "" {
		if _, err := os.Stat(tm.customPath); err == nil {
			return tm.customPath
		}
	}

	// Look in system PATH
	if p, err := exec.LookPath("cloudflared.exe"); err == nil {
		return p
	}
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p
	}

	// Look in common Windows installation directories
	userProfile := os.Getenv("USERPROFILE")
	localAppData := os.Getenv("LOCALAPPDATA")
	programFiles := os.Getenv("ProgramFiles")
	programFilesX86 := os.Getenv("ProgramFiles(x86)")
	programData := os.Getenv("ProgramData")

	candidates := []string{
		filepath.Join(programFiles, "cloudflared", "cloudflared.exe"),
		filepath.Join(programFilesX86, "cloudflared", "cloudflared.exe"),
		filepath.Join(programData, "chocolatey", "bin", "cloudflared.exe"),
		filepath.Join(localAppData, "cloudflared", "cloudflared.exe"),
		filepath.Join(userProfile, "bin", "cloudflared.exe"),
		filepath.Join(userProfile, "cloudflared.exe"),
		filepath.Join(userProfile, "scoop", "shims", "cloudflared.exe"),
	}

	for _, c := range candidates {
		if c != "" {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}

	return ""
}

// FindRunningTunnel checks if a cloudflared process running this specific tunnel is active
func (tm *TunnelManager) FindRunningTunnel() (bool, int) {
	// Query Win32_Process via PowerShell to inspect commandline args matching "tunnel run <name>"
	psScript := fmt.Sprintf(`$p = Get-CimInstance Win32_Process -Filter "Name = 'cloudflared.exe'" -ErrorAction SilentlyContinue | Where-Object { $_.CommandLine -like "*tunnel run %s*" } | Select-Object -First 1 -ExpandProperty ProcessId; if ($p) { Write-Output $p }`, tm.tunnelName)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.Output()
	if err == nil {
		strPID := strings.TrimSpace(string(out))
		if pid, err := strconv.Atoi(strPID); err == nil && pid > 0 {
			return true, pid
		}
	}

	return false, 0
}

// GetStatus returns the current tunnel status
func (tm *TunnelManager) GetStatus() TunnelStatus {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	binary := tm.FindCloudflared()
	installed := binary != ""

	running, pid := tm.FindRunningTunnel()
	if running {
		tm.lastPID = pid
	}

	msg := "Tunnel Stopped"
	if !installed {
		msg = "cloudflared executable not found"
	} else if running {
		msg = fmt.Sprintf("Tunnel '%s' running (PID: %d)", tm.tunnelName, pid)
	}

	return TunnelStatus{
		TunnelName: tm.tunnelName,
		BinaryPath: binary,
		Installed:  installed,
		Running:    running,
		PID:        pid,
		Message:    msg,
	}
}

// Start launches the Cloudflare tunnel in the background
func (tm *TunnelManager) Start() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	binary := tm.FindCloudflared()
	if binary == "" {
		return errors.New("cloudflared is not installed or not found in system PATH")
	}

	running, pid := tm.FindRunningTunnel()
	if running {
		return fmt.Errorf("tunnel '%s' is already running (PID: %d)", tm.tunnelName, pid)
	}

	cmd := exec.Command(binary, "tunnel", "run", tm.tunnelName)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start cloudflared tunnel: %w", err)
	}

	tm.lastPID = cmd.Process.Pid

	// Wait briefly and verify status
	time.Sleep(1 * time.Second)
	isRun, newPid := tm.FindRunningTunnel()
	if isRun {
		tm.lastPID = newPid
	}

	return nil
}

// Stop terminates the Cloudflare tunnel process for this tunnel
func (tm *TunnelManager) Stop() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Find all matching tunnel processes and kill them
	psScript := fmt.Sprintf(`Get-CimInstance Win32_Process -Filter "Name = 'cloudflared.exe'" -ErrorAction SilentlyContinue | Where-Object { $_.CommandLine -like "*tunnel run %s*" } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }`, tm.tunnelName)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop cloudflared tunnel: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	time.Sleep(500 * time.Millisecond)
	running, _ := tm.FindRunningTunnel()
	if running {
		return errors.New("tunnel process could not be terminated")
	}

	return nil
}
