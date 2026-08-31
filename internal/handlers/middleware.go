package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/skd19/vido-tunnel/internal/auth"
)

type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// SecurityHeadersMiddleware adds defensive HTTP security headers
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline' 'unsafe-eval'; img-src 'self' data:; font-src 'self' data:;")
		next.ServeHTTP(w, r)
	})
}

// LoggingMiddleware logs all incoming requests with IP and execution duration
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ip := auth.ExtractIP(r)

		wrapper := &responseWriterWrapper{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrapper, r)

		duration := time.Since(start)
		log.Printf("[%s] %s %s %d (%v)", ip, r.Method, r.URL.Path, wrapper.statusCode, duration)
	})
}

// RequireAuth creates a middleware ensuring the user is authenticated
func RequireAuth(authMgr *auth.Manager, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authMgr.IsAuthenticated(r) {
			if r.Method == http.MethodPost || r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
				http.Error(w, `{"error":"unauthorized","redirect":"/login"}`, http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}
