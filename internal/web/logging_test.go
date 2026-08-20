package web

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/store"
)

// captureLogs returns a logger writing JSON to buf at the given level, plus a decoder
// for the records it produced.
func captureLogs(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: level})), buf
}

func decodeRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line %q is not JSON: %v", line, err)
		}
		records = append(records, rec)
	}
	return records
}

func TestAccessLogRecordsRequest(t *testing.T) {
	logger, buf := captureLogs(slog.LevelInfo)
	h := AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("hola"))
	}))

	req := httptest.NewRequest("POST", "/productos/nuevo?q=secreto", nil)
	req.RemoteAddr = "192.168.3.20:51000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	records := decodeRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("got %d log records, want 1", len(records))
	}
	r := records[0]
	if r["msg"] != "request" || r["method"] != "POST" || r["status"] != float64(201) {
		t.Fatalf("record = %v", r)
	}
	if r["bytes"] != float64(4) || r["ip"] != "192.168.3.20" {
		t.Fatalf("record = %v", r)
	}
	if _, ok := r["duration_ms"]; !ok {
		t.Fatalf("record has no duration_ms: %v", r)
	}
	// The query string carries customer search text and must not be logged.
	if path, _ := r["path"].(string); path != "/productos/nuevo" {
		t.Fatalf("path = %q, want /productos/nuevo with no query", path)
	}
	if strings.Contains(buf.String(), "secreto") {
		t.Fatalf("query string leaked into the log: %s", buf.String())
	}
	// Unauthenticated: no user attributes at all, rather than a zero user.
	if _, ok := r["user_id"]; ok {
		t.Fatalf("anonymous request logged a user_id: %v", r)
	}
	if got := rec.Header().Get("X-Request-Id"); got == "" || got != r["request_id"] {
		t.Fatalf("X-Request-Id header %q does not match logged request_id %v", got, r["request_id"])
	}
}

func TestAccessLogReusesProxyRequestID(t *testing.T) {
	logger, buf := captureLogs(slog.LevelInfo)
	var seen string
	h := AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestID(r.Context())
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", "nginx-abc123")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if seen != "nginx-abc123" {
		t.Fatalf("RequestID = %q, want nginx-abc123", seen)
	}
	if decodeRecords(t, buf)[0]["request_id"] != "nginx-abc123" {
		t.Fatalf("logged request_id did not follow the proxy's")
	}
}

// A header carrying a newline would forge log lines and corrupt the echoed response
// header, so it is discarded in favour of a generated id.
func TestAccessLogRejectsMalformedProxyRequestID(t *testing.T) {
	logger, buf := captureLogs(slog.LevelInfo)
	h := AccessLog(logger)(okHandler())

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", "abc\ndef")
	h.ServeHTTP(httptest.NewRecorder(), req)

	got, _ := decodeRecords(t, buf)[0]["request_id"].(string)
	if got == "abc\ndef" || got == "" {
		t.Fatalf("request_id = %q, want a generated id", got)
	}
}

func TestAccessLogRecoversPanic(t *testing.T) {
	logger, buf := captureLogs(slog.LevelInfo)
	h := AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/productos", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	records := decodeRecords(t, buf)
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2 (panic + request)", len(records))
	}
	if records[0]["msg"] != "handler panic" || records[0]["level"] != "ERROR" {
		t.Fatalf("panic record = %v", records[0])
	}
	if stack, _ := records[0]["stack"].(string); !strings.Contains(stack, "logging_test.go") {
		t.Fatalf("panic record has no useful stack: %v", records[0]["stack"])
	}
	if records[1]["status"] != float64(500) || records[1]["level"] != "ERROR" {
		t.Fatalf("request record = %v", records[1])
	}
}

// Static assets and the deploy healthcheck are the two high-frequency paths; at Info
// they would drown everything else out.
func TestAccessLogFiltersNoiseByLevel(t *testing.T) {
	for _, path := range []string{"/static/app.css", "/healthz"} {
		logger, buf := captureLogs(slog.LevelInfo)
		AccessLog(logger)(okHandler()).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
		if buf.Len() != 0 {
			t.Fatalf("%s logged at Info: %s", path, buf.String())
		}

		debugLogger, debugBuf := captureLogs(slog.LevelDebug)
		AccessLog(debugLogger)(okHandler()).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
		if debugBuf.Len() == 0 {
			t.Fatalf("%s was not logged at Debug", path)
		}
	}
}

// A failing static request is not noise.
func TestAccessLogKeepsFailedStaticRequests(t *testing.T) {
	logger, buf := captureLogs(slog.LevelInfo)
	h := AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no existe", http.StatusNotFound)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/static/missing.css", nil))

	records := decodeRecords(t, buf)
	if len(records) != 1 || records[0]["level"] != "WARN" {
		t.Fatalf("records = %v", records)
	}
}

// The access log runs outermost, so it only learns who the user is because RequireAuth
// writes back into the requestInfo it planted.
func TestAccessLogNamesAuthenticatedUser(t *testing.T) {
	a := newTestAuth(t)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "contraseña")
	token, err := randomToken()
	if err != nil {
		t.Fatalf("randomToken: %v", err)
	}
	if err := a.store.CreateSession(context.Background(), userID, hashToken(token)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	logger, buf := captureLogs(slog.LevelInfo)
	h := AccessLog(logger)(a.RequireAuth(okHandler()))

	req := httptest.NewRequest("GET", "/productos", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	h.ServeHTTP(httptest.NewRecorder(), req)

	r := decodeRecords(t, buf)[0]
	if r["user_id"] != float64(userID) || r["user"] != "rodolfo" {
		t.Fatalf("record = %v, want user rodolfo (%d)", r, userID)
	}
}

// RequireAuth also has to attribute writes, so an edit made through the web UI lands in
// the audit trail under the right user.
func TestRequireAuthAttributesStoreWrites(t *testing.T) {
	a := newTestAuth(t)
	userID := createTestUser(t, a, "rodolfo", "admin", "contraseña")
	token, err := randomToken()
	if err != nil {
		t.Fatalf("randomToken: %v", err)
	}
	if err := a.store.CreateSession(context.Background(), userID, hashToken(token)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	h := a.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := a.store.SetSetting(r.Context(), "fx_rate_micros", "18000000", userID); err != nil {
			t.Errorf("SetSetting: %v", err)
		}
	}))
	req := httptest.NewRequest("POST", "/ajustes", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	h.ServeHTTP(httptest.NewRecorder(), req)

	entries, err := a.store.AuditLog(context.Background(), store.AuditFilter{TableName: "settings"})
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d audit entries, want 1", len(entries))
	}
	if entries[0].ActorID == nil || *entries[0].ActorID != userID {
		t.Fatalf("audit actor = %v, want %d", entries[0].ActorID, userID)
	}
	if entries[0].Source != store.SourceWeb {
		t.Fatalf("audit source = %q, want %q", entries[0].Source, store.SourceWeb)
	}
}
