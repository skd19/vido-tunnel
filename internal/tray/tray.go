package tray

import (
	"fmt"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	shell32                    = syscall.NewLazyDLL("shell32.dll")
	procRegisterClassExW       = user32.NewProc("RegisterClassExW")
	procCreateWindowExW        = user32.NewProc("CreateWindowExW")
	procDefWindowProcW         = user32.NewProc("DefWindowProcW")
	procDestroyWindow          = user32.NewProc("DestroyWindow")
	procPostQuitMessage        = user32.NewProc("PostQuitMessage")
	procGetMessageW            = user32.NewProc("GetMessageW")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procDispatchMessageW       = user32.NewProc("DispatchMessageW")
	procCreatePopupMenu        = user32.NewProc("CreatePopupMenu")
	procAppendMenuW            = user32.NewProc("AppendMenuW")
	procTrackPopupMenu         = user32.NewProc("TrackPopupMenu")
	procSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	procGetCursorPos           = user32.NewProc("GetCursorPos")
	procPostMessageW           = user32.NewProc("PostMessageW")
	procShellNotifyIconW       = shell32.NewProc("Shell_NotifyIconW")
	procLoadIconW              = user32.NewProc("LoadIconW")
)

const (
	WM_USER         = 0x0400
	WM_TRAYICON     = WM_USER + 1
	WM_DESTROY      = 0x0002
	WM_COMMAND      = 0x0111
	WM_LBUTTONDBLCLK= 0x0203
	WM_RBUTTONUP    = 0x0205
	WM_LBUTTONUP    = 0x0202
	WM_NULL         = 0x0000

	NIM_ADD         = 0x00000000
	NIM_MODIFY      = 0x00000001
	NIM_DELETE      = 0x00000002

	NIF_MESSAGE     = 0x00000001
	NIF_ICON        = 0x00000002
	NIF_TIP         = 0x00000004

	MF_STRING       = 0x00000000
	MF_SEPARATOR    = 0x00000800
	TPM_BOTTOMALIGN = 0x0020
	TPM_RIGHTALIGN  = 0x0008

	IDI_APPLICATION = 32512
	IDI_SHIELD      = 32518

	ID_OPEN_DASHBOARD = 1001
	ID_OPEN_CONTROL   = 1002
	ID_OPEN_FOLDER    = 1003
	ID_VIDOVEO_START  = 1004
	ID_VIDOVEO_STOP   = 1005
	ID_TUNNEL_RESTART = 1006
	ID_EXIT           = 1007
)

type POINT struct {
	X, Y int32
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

type NOTIFYICONDATAW struct {
	CbSize           uint32
	HWnd             syscall.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            syscall.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeoutOrVersion uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     syscall.Handle
}

type MSG struct {
	HWnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

// Config for Tray actions
type Config struct {
	Title           string
	DashboardURL    string
	ControlURL      string
	StoragePath     string
	OnStartVidoveo  func()
	OnStopVidoveo   func()
	OnRestartTunnel func()
	OnExit          func()
}

type App struct {
	cfg   Config
	hwnd  syscall.Handle
	nid   NOTIFYICONDATAW
	hMenu syscall.Handle
}

var currentApp *App

// New creates a new System Tray manager
func New(cfg Config) *App {
	return &App{
		cfg: cfg,
	}
}

// Run creates the hidden window, registers the tray icon, and runs the message loop
func (a *App) Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	currentApp = a

	className, err := syscall.UTF16PtrFromString("VidoTunnelTrayClass")
	if err != nil {
		return err
	}

	windowName, err := syscall.UTF16PtrFromString("VidoTunnelTrayWindow")
	if err != nil {
		return err
	}

	hInst := syscall.Handle(0)
	hIcon, _, _ := procLoadIconW.Call(0, uintptr(IDI_SHIELD))
	if hIcon == 0 {
		hIcon, _, _ = procLoadIconW.Call(0, uintptr(IDI_APPLICATION))
	}

	wndProcCallback := syscall.NewCallback(wndProc)

	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		LpfnWndProc:   wndProcCallback,
		HInstance:     hInst,
		HIcon:         syscall.Handle(hIcon),
		LpszClassName: className,
	}

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0,
		0, 0, 0, 0,
		0, 0, uintptr(hInst), 0,
	)

	if hwnd == 0 {
		return fmt.Errorf("failed to create tray window")
	}

	a.hwnd = syscall.Handle(hwnd)

	// Setup Notify Icon Data
	a.nid = NOTIFYICONDATAW{
		CbSize:           uint32(unsafe.Sizeof(NOTIFYICONDATAW{})),
		HWnd:             a.hwnd,
		UID:              1,
		UFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
		UCallbackMessage: WM_TRAYICON,
		HIcon:            syscall.Handle(hIcon),
	}

	tip := a.cfg.Title
	if tip == "" {
		tip = "Vido Tunnel"
	}
	tipW, _ := syscall.UTF16FromString(tip)
	copy(a.nid.SzTip[:], tipW)

	// Add icon to system tray
	procShellNotifyIconW.Call(uintptr(NIM_ADD), uintptr(unsafe.Pointer(&a.nid)))

	// Run message pump
	var msg MSG
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	// Remove icon on exit
	procShellNotifyIconW.Call(uintptr(NIM_DELETE), uintptr(unsafe.Pointer(&a.nid)))
	return nil
}

// Stop closes the tray app and posts WM_QUIT
func (a *App) Stop() {
	if a.hwnd != 0 {
		procShellNotifyIconW.Call(uintptr(NIM_DELETE), uintptr(unsafe.Pointer(&a.nid)))
		procPostQuitMessage.Call(0)
	}
}

func wndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	if currentApp == nil {
		r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return r
	}

	switch msg {
	case WM_TRAYICON:
		switch lParam {
		case WM_RBUTTONUP:
			currentApp.showContextMenu()
		case WM_LBUTTONDBLCLK:
			OpenURL(currentApp.cfg.DashboardURL)
		}
		return 0

	case WM_COMMAND:
		cmdID := int(wParam & 0xFFFF)
		switch cmdID {
		case ID_OPEN_DASHBOARD:
			OpenURL(currentApp.cfg.DashboardURL)
		case ID_OPEN_CONTROL:
			OpenURL(currentApp.cfg.ControlURL)
		case ID_OPEN_FOLDER:
			OpenFolder(currentApp.cfg.StoragePath)
		case ID_VIDOVEO_START:
			if currentApp.cfg.OnStartVidoveo != nil {
				go currentApp.cfg.OnStartVidoveo()
			}
		case ID_VIDOVEO_STOP:
			if currentApp.cfg.OnStopVidoveo != nil {
				go currentApp.cfg.OnStopVidoveo()
			}
		case ID_TUNNEL_RESTART:
			if currentApp.cfg.OnRestartTunnel != nil {
				go currentApp.cfg.OnRestartTunnel()
			}
		case ID_EXIT:
			if currentApp.cfg.OnExit != nil {
				go currentApp.cfg.OnExit()
			}
			currentApp.Stop()
		}
		return 0

	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	}

	r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

func (a *App) showContextMenu() {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}

	dashStr, _ := syscall.UTF16PtrFromString("Open Dashboard")
	ctrlStr, _ := syscall.UTF16PtrFromString("Open Control Panel")
	folderStr, _ := syscall.UTF16PtrFromString("Open Storage Folder")
	startVidStr, _ := syscall.UTF16PtrFromString("Start Vidoveo")
	stopVidStr, _ := syscall.UTF16PtrFromString("Stop Vidoveo")
	restartTunStr, _ := syscall.UTF16PtrFromString("Restart Cloudflare Tunnel")
	exitStr, _ := syscall.UTF16PtrFromString("Exit Vido Tunnel")

	procAppendMenuW.Call(hMenu, uintptr(MF_STRING), uintptr(ID_OPEN_DASHBOARD), uintptr(unsafe.Pointer(dashStr)))
	procAppendMenuW.Call(hMenu, uintptr(MF_STRING), uintptr(ID_OPEN_CONTROL), uintptr(unsafe.Pointer(ctrlStr)))
	procAppendMenuW.Call(hMenu, uintptr(MF_STRING), uintptr(ID_OPEN_FOLDER), uintptr(unsafe.Pointer(folderStr)))
	procAppendMenuW.Call(hMenu, uintptr(MF_SEPARATOR), 0, 0)
	procAppendMenuW.Call(hMenu, uintptr(MF_STRING), uintptr(ID_VIDOVEO_START), uintptr(unsafe.Pointer(startVidStr)))
	procAppendMenuW.Call(hMenu, uintptr(MF_STRING), uintptr(ID_VIDOVEO_STOP), uintptr(unsafe.Pointer(stopVidStr)))
	procAppendMenuW.Call(hMenu, uintptr(MF_STRING), uintptr(ID_TUNNEL_RESTART), uintptr(unsafe.Pointer(restartTunStr)))
	procAppendMenuW.Call(hMenu, uintptr(MF_SEPARATOR), 0, 0)
	procAppendMenuW.Call(hMenu, uintptr(MF_STRING), uintptr(ID_EXIT), uintptr(unsafe.Pointer(exitStr)))

	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	procSetForegroundWindow.Call(uintptr(a.hwnd))
	procTrackPopupMenu.Call(
		hMenu,
		uintptr(TPM_RIGHTALIGN|TPM_BOTTOMALIGN),
		uintptr(pt.X),
		uintptr(pt.Y),
		0,
		uintptr(a.hwnd),
		0,
	)
	procPostMessageW.Call(uintptr(a.hwnd), uintptr(WM_NULL), 0, 0)
}

// OpenURL opens a URL in the user's default web browser
func OpenURL(targetURL string) {
	if targetURL == "" {
		return
	}
	cmd := exec.Command("cmd.exe", "/c", "start", "", targetURL)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
}

// OpenFolder opens a folder path in Windows Explorer
func OpenFolder(dirPath string) {
	if dirPath == "" {
		return
	}
	cmd := exec.Command("explorer.exe", dirPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
}
