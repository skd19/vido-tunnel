package auth

import (
	"testing"
	"time"
)

func TestAuthManager_ValidateKey(t *testing.T) {
	secret := "super-secure-key-12345"
	sessionKey := []byte("12345678901234567890123456789012")

	mgr := NewManager(secret, sessionKey)

	if !mgr.ValidateKey("super-secure-key-12345") {
		t.Errorf("ValidateKey failed for correct key")
	}

	if mgr.ValidateKey("wrong-key") {
		t.Errorf("ValidateKey passed for wrong key")
	}

	if mgr.ValidateKey("") {
		t.Errorf("ValidateKey passed for empty key")
	}
}

func TestAuthManager_SessionToken(t *testing.T) {
	secret := "secret-key"
	sessionKey := []byte("12345678901234567890123456789012")

	mgr := NewManager(secret, sessionKey)

	token := mgr.CreateSessionToken()
	if !mgr.ValidateSessionToken(token) {
		t.Errorf("ValidateSessionToken failed for valid token")
	}

	// Tampered token
	tamperedToken := token + "tampered"
	if mgr.ValidateSessionToken(tamperedToken) {
		t.Errorf("ValidateSessionToken passed for tampered token")
	}

	// Different key manager
	otherMgr := NewManager(secret, []byte("othersecretkey123456789012345678"))
	if otherMgr.ValidateSessionToken(token) {
		t.Errorf("ValidateSessionToken passed with different session secret")
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(3, 100*time.Millisecond)
	ip := "192.168.1.100"

	// 1st attempt
	locked, _ := rl.RecordFailure(ip)
	if locked {
		t.Errorf("Locked out after 1 failure")
	}

	// 2nd attempt
	locked, _ = rl.RecordFailure(ip)
	if locked {
		t.Errorf("Locked out after 2 failures")
	}

	// 3rd attempt -> should lock
	locked, wait := rl.RecordFailure(ip)
	if !locked || wait <= 0 {
		t.Errorf("Expected lockout after 3 failures")
	}

	// Check IsAllowed
	allowed, _ := rl.IsAllowed(ip)
	if allowed {
		t.Errorf("IsAllowed returned true during lockout")
	}

	// Wait for lock duration
	time.Sleep(120 * time.Millisecond)

	allowed, _ = rl.IsAllowed(ip)
	if !allowed {
		t.Errorf("IsAllowed returned false after lockout expired")
	}

	// Test RecordSuccess
	rl.RecordSuccess(ip)
	allowed, _ = rl.IsAllowed(ip)
	if !allowed {
		t.Errorf("IsAllowed returned false after RecordSuccess")
	}
}
