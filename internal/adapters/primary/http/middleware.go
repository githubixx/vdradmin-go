package http

import (
	"compress/gzip"
	"context"
	"crypto/subtle"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
)

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

// LoggingMiddleware logs HTTP requests
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rw := &responseWriter{ResponseWriter: w}
			next.ServeHTTP(rw, r)

			duration := time.Since(start)

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote", r.RemoteAddr),
				slog.Int("status", rw.status),
				slog.Int("size", rw.size),
				slog.Duration("duration", duration),
			}

			// HLS clients can legitimately request segments that are already gone (or not yet present)
			// during stream startup/channel switching. Avoid spamming logs with these expected misses.
			if strings.HasPrefix(r.URL.Path, "/watch/stream/") {
				switch rw.status {
				case http.StatusNotFound, http.StatusServiceUnavailable:
					logger.LogAttrs(r.Context(), slog.LevelDebug, "request", attrs...)
					return
				}
			}

			logger.LogAttrs(r.Context(), slog.LevelInfo, "request", attrs...)
		})
	}
}

// RecoveryMiddleware recovers from panics
func RecoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("panic recovered",
						slog.Any("error", err),
						slog.String("path", r.URL.Path),
					)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// AuthMiddleware handles authentication
func AuthMiddleware(cfg *config.AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			// Always treat loopback as trusted. This avoids external players (VLC/mpv)
			// repeatedly prompting for credentials when opening localhost URLs.
			if isLoopbackRemote(r.RemoteAddr) {
				ctx := context.WithValue(r.Context(), "user", cfg.AdminUser)
				ctx = context.WithValue(ctx, "role", "admin")
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Check if request is from local network
			if isLocalNet(r.RemoteAddr, cfg.LocalNets) {
				ctx := context.WithValue(r.Context(), "user", cfg.AdminUser)
				ctx = context.WithValue(ctx, "role", "admin")
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Check Basic Auth
			user, pass, ok := r.BasicAuth()
			if !ok {
				w.Header().Set("WWW-Authenticate", `Basic realm="VDRAdmin"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Verify credentials
			role := ""
			if secureCompare(user, cfg.AdminUser) && secureCompare(pass, cfg.AdminPass) {
				role = "admin"
			} else if cfg.GuestEnabled && secureCompare(user, cfg.GuestUser) && secureCompare(pass, cfg.GuestPass) {
				role = "guest"
			} else {
				w.Header().Set("WWW-Authenticate", `Basic realm="VDRAdmin"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Add user info to context
			ctx := context.WithValue(r.Context(), "user", user)
			ctx = context.WithValue(ctx, "role", role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdminMiddleware ensures user has admin role
func RequireAdminMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := r.Context().Value("role").(string)
			if !ok || role != "admin" {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CompressionMiddleware handles gzip compression
func CompressionMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if client accepts gzip
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				next.ServeHTTP(w, r)
				return
			}

			// Create gzip writer
			gz := gzip.NewWriter(w)
			defer gz.Close()

			// Wrap response writer
			w.Header().Set("Content-Encoding", "gzip")
			gzw := &gzipResponseWriter{Writer: gz, ResponseWriter: w}

			next.ServeHTTP(gzw, r)
		})
	}
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// CORSMiddleware adds CORS headers
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && isAllowedOrigin(origin, allowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeadersMiddleware adds security headers
func SecurityHeadersMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			next.ServeHTTP(w, r)
		})
	}
}

// Helper functions

func secureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func isLocalNet(remoteAddr string, localNets []string) bool {
	if len(localNets) == 0 {
		return false
	}

	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	for _, cidr := range localNets {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if ipNet.Contains(ip) {
			return true
		}
	}

	return false
}

func isLoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

func isAllowedOrigin(origin string, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}

	for _, o := range allowed {
		if o == "*" || o == origin {
			return true
		}
	}

	return false
}
