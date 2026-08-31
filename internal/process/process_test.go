package process

import (
	"fmt"
	"net"
	"testing"
)

func TestCheckPort(t *testing.T) {
	// Start a local dummy TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start test listener: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	mgr := NewManager("dummy.exe", port)

	if !mgr.CheckPort(port) {
		t.Errorf("CheckPort failed for open port %d", port)
	}

	// Test an unopened port (e.g. port + 1)
	if mgr.CheckPort(port + 1999) {
		t.Errorf("CheckPort returned true for closed port %d", port+1999)
	}
}

func TestGetStatus(t *testing.T) {
	mgr := NewManager("C:\\Vidoveo\\Vidoveo.exe", 7788)
	status := mgr.GetStatus()

	if status.Name != "Vidoveo.exe" {
		t.Errorf("GetStatus Name = %s, want Vidoveo.exe", status.Name)
	}
	if status.Port != 7788 {
		t.Errorf("GetStatus Port = %d, want 7788", status.Port)
	}
	if status.LastChecked == "" {
		t.Errorf("GetStatus LastChecked is empty")
	}
	fmt.Printf("Process Status Initial: %+v\n", status)
}
