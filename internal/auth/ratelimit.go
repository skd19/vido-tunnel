package auth

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ipRecord struct {
	failedAttempts int
	lockedUntil    time.Time
	lastAttempt    time.Time
}

// RateLimiter protects against brute force attacks on authentication
type RateLimiter struct {
	mu           sync.Mutex
	records      map[string]*ipRecord
	maxAttempts  int
	lockDuration time.Duration
}

// NewRateLimiter creates a new RateLimiter instance
func NewRateLimiter(maxAttempts int, lockDuration time.Duration) *RateLimiter {
	rl := &RateLimiter{
		records:      make(map[string]*ipRecord),
		maxAttempts:  maxAttempts,
		lockDuration: lockDuration,
	}

	// Background cleanup of stale entries every 10 minutes
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		for range ticker.C {
			rl.cleanup()
		}
	}()

	return rl
}

// ExtractIP gets the client IP from request headers or RemoteAddr
func ExtractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// IsAllowed checks if an IP is currently blocked
func (rl *RateLimiter) IsAllowed(ip string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rec, exists := rl.records[ip]
	if !exists {
		return true, 0
	}

	if time.Now().Before(rec.lockedUntil) {
		return false, time.Until(rec.lockedUntil)
	}

	return true, 0
}

// RecordFailure increments failed attempts and locks out if threshold exceeded
func (rl *RateLimiter) RecordFailure(ip string) (locked bool, remainingWait time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rec, exists := rl.records[ip]
	now := time.Now()

	if !exists {
		rec = &ipRecord{
			failedAttempts: 1,
			lastAttempt:    now,
		}
		rl.records[ip] = rec
		return false, 0
	}

	// Reset attempts if last attempt was over 10 minutes ago
	if now.Sub(rec.lastAttempt) > 10*time.Minute {
		rec.failedAttempts = 1
		rec.lastAttempt = now
		rec.lockedUntil = time.Time{}
		return false, 0
	}

	rec.failedAttempts++
	rec.lastAttempt = now

	if rec.failedAttempts >= rl.maxAttempts {
		rec.lockedUntil = now.Add(rl.lockDuration)
		return true, rl.lockDuration
	}

	return false, 0
}

// RecordSuccess clears failure records for the IP
func (rl *RateLimiter) RecordSuccess(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.records, ip)
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, rec := range rl.records {
		if now.Sub(rec.lastAttempt) > 30*time.Minute && now.After(rec.lockedUntil) {
			delete(rl.records, ip)
		}
	}
}
