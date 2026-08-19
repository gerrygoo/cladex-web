package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
)

// requestInfo carries per-request facts that are only known part-way through the
// chain. The access-log middleware runs outermost, so it cannot see the user that
// RequireAuth resolves further in; it plants this pointer on the way in and reads it
// back on the way out.
type requestInfo struct {
	id       string
	userID   int64
	username string
}

var requestInfoKey = &contextKey{"requestInfo"}

func withRequestInfo(ctx context.Context, info *requestInfo) context.Context {
	return context.WithValue(ctx, requestInfoKey, info)
}

func requestInfoFrom(ctx context.Context) *requestInfo {
	info, _ := ctx.Value(requestInfoKey).(*requestInfo)
	return info
}

// RequestID returns the id assigned to this request by AccessLog, or "" outside a
// request. It matches the X-Request-Id response header, so a user reporting an error can
// be tied to a log line.
func RequestID(ctx context.Context) string {
	if info := requestInfoFrom(ctx); info != nil {
		return info.id
	}
	return ""
}

// AccessLog logs one structured line per request and turns a handler panic into a 500
// instead of a silently dropped connection.
//
// Only metadata is logged, never bodies: POST /login carries a plaintext password and
// POST /clientes carries customer PII. Query strings are omitted for the same reason —
// the ?q= on the list pages is customer search text.
func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := &requestInfo{id: requestIDFor(r)}
			r = r.WithContext(withRequestInfo(r.Context(), info))
			w.Header().Set("X-Request-Id", info.id)

			start := time.Now()
			var m httpsnoop.Metrics
			panicked := false

			defer func() {
				if rec := recover(); rec != nil {
					panicked = true
					logger.LogAttrs(r.Context(), slog.LevelError, "handler panic",
						slog.String("request_id", info.id),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
						slog.Any("panic", rec),
						slog.String("stack", string(debug.Stack())),
					)
					// Best-effort: if the handler already wrote a header this is a no-op
					// beyond a "superfluous WriteHeader" notice.
					http.Error(w, "error interno", http.StatusInternalServerError)
					m.Code = http.StatusInternalServerError
				}

				attrs := []slog.Attr{
					slog.String("request_id", info.id),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", m.Code),
					slog.Int64("duration_ms", time.Since(start).Milliseconds()),
					slog.Int64("bytes", m.Written),
					slog.String("ip", clientIP(r)),
				}
				if info.userID != 0 {
					attrs = append(attrs,
						slog.Int64("user_id", info.userID),
						slog.String("user", info.username))
				}
				logger.LogAttrs(r.Context(), levelFor(r, m.Code, panicked), "request", attrs...)
			}()

			m = httpsnoop.CaptureMetrics(next, w, r)
		})
	}
}

// levelFor keeps the log readable at Info: server errors stand out, and the two sources
// of steady background noise — embedded static assets and the deploy healthcheck, which
// the NAS polls every 5 minutes — drop to Debug unless they fail.
func levelFor(r *http.Request, status int, panicked bool) slog.Level {
	switch {
	case panicked || status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	case status < 400 && (strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/healthz"):
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

// requestIDFor reuses the id nginx assigns if there is one, so a request can be followed
// across the proxy, and mints one otherwise.
func requestIDFor(r *http.Request) string {
	if id := r.Header.Get("X-Request-Id"); id != "" && len(id) <= 64 && isPrintableASCII(id) {
		return id
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

// isPrintableASCII rejects a header value that would corrupt the log stream or the
// X-Request-Id response header it gets echoed into.
func isPrintableASCII(s string) bool {
	for _, c := range []byte(s) {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}
