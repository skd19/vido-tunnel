package tray

import (
	"testing"
)

func TestCopyToClipboard(t *testing.T) {
	testSecret := "my-sample-secret-key-123"
	err := CopyToClipboard(testSecret)
	if err != nil {
		t.Fatalf("CopyToClipboard returned error: %v", err)
	}
}
