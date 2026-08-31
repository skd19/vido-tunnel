package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	CookieName    = "vido_session"
	SessionMaxAge = 7 * 24 * time.Hour // 7 days
)

// Manager handles secret key verification and HMAC session cookies
type Manager struct {
	secretKey     string
	sessionSecret []byte
}

// NewManager creates a new Auth Manager
func NewManager(secretKey string, sessionSecret []byte) *Manager {
	return &Manager{
		secretKey:     secretKey,
		sessionSecret: sessionSecret,
	}
}

// ValidateKey compares the provided key against the configured secret key in constant time
func (m *Manager) ValidateKey(providedKey string) bool {
	if providedKey == "" || m.secretKey == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(providedKey), []byte(m.secretKey)) == 1
}

// CreateSessionToken generates a timestamped HMAC-signed token
func (m *Manager) CreateSessionToken() string {
	now := time.Now().Unix()
	msg := fmt.Sprintf("auth:%d", now)

	h := hmac.New(sha256.New, m.sessionSecret)
	h.Write([]byte(msg))
	sig := hex.EncodeToString(h.Sum(nil))

	return fmt.Sprintf("%d:%s", now, sig)
}

// ValidateSessionToken checks if the session token is valid and not expired
func (m *Manager) ValidateSessionToken(token string) bool {
	parts := strings.Split(token, ":")
	if len(parts) != 2 {
		return false
	}

	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}

	tokenTime := time.Unix(ts, 0)
	if time.Since(tokenTime) > SessionMaxAge || time.Until(tokenTime) > 1*time.Hour {
		return false
	}

	msg := fmt.Sprintf("auth:%d", ts)
	h := hmac.New(sha256.New, m.sessionSecret)
	h.Write([]byte(msg))
	expectedSig := hex.EncodeToString(h.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(expectedSig)) == 1
}

// SetSessionCookie sets a secure, HTTP-only session cookie
func (m *Manager) SetSessionCookie(w http.ResponseWriter, r *http.Request) {
	token := m.CreateSessionToken()
	cookie := &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(SessionMaxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	}
	http.SetCookie(w, cookie)
}

// ClearSessionCookie invalidates the session cookie
func (m *Manager) ClearSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)
}

// IsAuthenticated checks if the request has a valid session cookie
func (m *Manager) IsAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(CookieName)
	if err != nil || cookie == nil {
		return false
	}
	return m.ValidateSessionToken(cookie.Value)
}
