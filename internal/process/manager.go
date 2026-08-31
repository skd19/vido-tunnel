package process

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ProcessStatus struct {
	Name        string `json:"name"`
	ExecPath    string `json:"exec_path"`
	PathExists  bool   `json:"path_exists"`
	Running     bool   `json:"running"`
	PID         int    `json:"pid"`
	Port        int    `json:"port"`
	PortOpen    bool   `json:"port_open"`
	LastChecked string `json:"last_checked"`
	Message     string `json:"message"`
}

type Manager struct {
	mu         sync.Mutex
	execPath   string
	targetPort int
	lastPID    int
}

// NewManager creates a new process supervisor for Vidoveo
func NewManager(execPath string, targetPort int) *Manager {
	return &Manager{
		execPath:   execPath,
		targetPort: targetPort,
	}
}

// CheckPort tests whether the target TCP port is open on localhost
func (m *Manager) CheckPort(port int) bool {
	address := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", address, 600*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// FindRunningProcess checks if the executable is currently running and returns its PID
func (m *Manager) FindRunningProcess() (bool, int) {
	exeName := filepath.Base(m.execPath)

	// Run tasklist on Windows
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", exeName), "/FO", "CSV", "/NH")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.Output()
	if err != nil {
		return false, 0
	}

	reader := csv.NewReader(bytes.NewReader(out))
	records, err := reader.ReadAll()
	if err != nil {
		return false, 0
	}

	for _, rec := range records {
		if len(rec) >= 2 {
			imgName := strings.TrimSpace(rec[0])
			if strings.EqualFold(imgName, exeName) {
				if pid, err := strconv.Atoi(strings.TrimSpace(rec[1])); err == nil && pid > 0 {
					return true, pid
				}
			}
		}
	}

	return false, 0
}

// GetStatus returns the current application and port status
func (m *Manager) GetStatus() ProcessStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, pathErr := os.Stat(m.execPath)
	pathExists := !os.IsNotExist(pathErr)

	running, pid := m.FindRunningProcess()
	if running {
		m.lastPID = pid
	}

	portOpen := m.CheckPort(m.targetPort)

	statusMsg := "Ready"
	if !pathExists {
		statusMsg = fmt.Sprintf("Executable not found at '%s'", m.execPath)
	} else if running && portOpen {
		statusMsg = "Running & Port Open"
	} else if running && !portOpen {
		statusMsg = "Running (Port Closed / Initializing)"
	} else if !running && portOpen {
		statusMsg = "Stopped (Port in use by another service)"
	} else {
		statusMsg = "Stopped"
	}

	return ProcessStatus{
		Name:        filepath.Base(m.execPath),
		ExecPath:    m.execPath,
		PathExists:  pathExists,
		Running:     running,
		PID:         pid,
		Port:        m.targetPort,
		PortOpen:    portOpen,
		LastChecked: time.Now().Format("15:04:05"),
		Message:     statusMsg,
	}
}

// Start launches the executable in the background detached from the server
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := os.Stat(m.execPath); os.IsNotExist(err) {
		return fmt.Errorf("executable does not exist at: %s", m.execPath)
	}

	running, pid := m.FindRunningProcess()
	if running {
		return fmt.Errorf("process is already running (PID: %d)", pid)
	}

	workDir := filepath.Dir(m.execPath)
	cmd := exec.Command(m.execPath)
	cmd.Dir = workDir
	// Detach process on Windows so it doesn't close with our console or block
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x08000000, // CREATE_NO_WINDOW
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s: %w", filepath.Base(m.execPath), err)
	}

	m.lastPID = cmd.Process.Pid

	// Give it a brief moment to initialize
	time.Sleep(300 * time.Millisecond)
	return nil
}

// Stop terminates the running executable
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	running, pid := m.FindRunningProcess()
	if !running {
		return errors.New("process is not running")
	}

	exeName := filepath.Base(m.execPath)

	// Attempt taskkill on Windows
	killCmd := exec.Command("taskkill", "/F", "/IM", exeName)
	killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := killCmd.Run(); err != nil {
		// Fallback: kill by PID if taskkill by image failed
		if pid > 0 {
			killPidCmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
			killPidCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			if errPid := killPidCmd.Run(); errPid != nil {
				return fmt.Errorf("failed to stop process (PID %d): %w", pid, errPid)
			}
		} else {
			return fmt.Errorf("failed to terminate %s: %w", exeName, err)
		}
	}

	// Verify termination
	time.Sleep(300 * time.Millisecond)
	stillRunning, _ := m.FindRunningProcess()
	if stillRunning {
		return errors.New("process failed to terminate completely")
	}

	return nil
}
