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
	"unsafe"
)

var (
	user32                   = syscall.NewLazyDLL("user32.dll")
	procEnumWindows          = user32.NewProc("EnumWindows")
	procGetWindowThreadProc  = user32.NewProc("GetWindowThreadProcessId")
	procShowWindowAsync      = user32.NewProc("ShowWindowAsync")
	procIsWindowVisible      = user32.NewProc("IsWindowVisible")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
)

const (
	SW_HIDE            = 0
	SW_SHOWMINIMIZED   = 2
	SW_MINIMIZE        = 6
	SW_SHOWMINNOACTIVE = 7
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

// FindAllRunningPIDs returns all PIDs matching the executable name
func (m *Manager) FindAllRunningPIDs() []int {
	exeName := filepath.Base(m.execPath)

	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", exeName), "/FO", "CSV", "/NH")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	reader := csv.NewReader(bytes.NewReader(out))
	records, err := reader.ReadAll()
	if err != nil {
		return nil
	}

	var pids []int
	for _, rec := range records {
		if len(rec) >= 2 {
			imgName := strings.TrimSpace(rec[0])
			if strings.EqualFold(imgName, exeName) {
				if pid, err := strconv.Atoi(strings.TrimSpace(rec[1])); err == nil && pid > 0 {
					pids = append(pids, pid)
				}
			}
		}
	}
	return pids
}

// FindRunningProcess checks if the executable is currently running and returns its primary PID
func (m *Manager) FindRunningProcess() (bool, int) {
	pids := m.FindAllRunningPIDs()
	if len(pids) > 0 {
		return true, pids[0]
	}
	return false, 0
}

// MinimizeWindows scans top-level windows and minimizes all windows belonging to Vidoveo processes
func (m *Manager) MinimizeWindows() {
	pids := m.FindAllRunningPIDs()
	pidMap := make(map[uint32]bool)
	for _, p := range pids {
		pidMap[uint32(p)] = true
	}
	if m.lastPID > 0 {
		pidMap[uint32(m.lastPID)] = true
	}

	exeBase := strings.ToLower(strings.TrimSuffix(filepath.Base(m.execPath), filepath.Ext(m.execPath)))

	cb := syscall.NewCallback(func(hwnd syscall.Handle, lParam uintptr) uintptr {
		vis, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
		if vis == 0 {
			return 1
		}

		var windowPID uint32
		procGetWindowThreadProc.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&windowPID)))

		shouldMinimize := pidMap[windowPID]

		// Also check window title if title contains the application name
		if !shouldMinimize {
			length, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd))
			if length > 0 {
				buf := make([]uint16, length+1)
				procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(length+1))
				title := strings.ToLower(syscall.UTF16ToString(buf))
				if strings.Contains(title, exeBase) {
					shouldMinimize = true
				}
			}
		}

		if shouldMinimize {
			// Minimize window without stealing focus or activating it to the front
			procShowWindowAsync.Call(uintptr(hwnd), uintptr(SW_SHOWMINNOACTIVE))
		}

		return 1
	})

	procEnumWindows.Call(cb, 0)
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

// Start launches the executable and minimizes all spawned windows (including PyInstaller child windows)
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
	// Launch with start /min in cmd so the initial process starts minimized
	cmd := exec.Command("cmd.exe", "/c", "start", "", "/min", m.execPath)
	cmd.Dir = workDir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s: %w", filepath.Base(m.execPath), err)
	}

	// Watchdog goroutine: for PyInstaller apps that extract and spawn a 2nd console or GUI window,
	// continuously detect and minimize all spawned windows for 15 seconds after launch.
	go func() {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			m.MinimizeWindows()
			time.Sleep(200 * time.Millisecond)
		}
	}()

	// Give the process a moment to initialize and capture PID
	time.Sleep(500 * time.Millisecond)
	if isRun, newPid := m.FindRunningProcess(); isRun {
		m.lastPID = newPid
	}

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

	// Attempt taskkill on Windows to kill process tree (/T) and force (/F)
	killCmd := exec.Command("taskkill", "/F", "/T", "/IM", exeName)
	killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := killCmd.Run(); err != nil {
		// Fallback: kill by PID if taskkill by image failed
		if pid > 0 {
			killPidCmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
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
