package process

import (
	"testing"
)

func TestTunnelManager_GetStatus(t *testing.T) {
	tm := NewTunnelManager("vidoveo", "")
	status := tm.GetStatus()

	if status.TunnelName != "vidoveo" {
		t.Errorf("TunnelName = %s, want vidoveo", status.TunnelName)
	}
	if status.Message == "" {
		t.Errorf("Tunnel message is empty")
	}
}

func TestTunnelManager_CustomPath(t *testing.T) {
	tm := NewTunnelManager("custom-tunnel", "C:\\nonexistent\\cloudflared.exe")
	status := tm.GetStatus()

	if status.Installed {
		t.Errorf("Expected installed=false for nonexistent path")
	}
}
