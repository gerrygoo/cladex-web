package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/store"
)

func newTestAuth(t *testing.T) *Auth {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return NewAuth(s, false)
}

func createTestUser(t *testing.T, a *Auth, username, role, password string) int64 {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	id, err := a.store.CreateUser(context.Background(), username, username, string(hash), role)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return id
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireAuthNoCookieRedirects(t *testing.T) {
	a := newTestAuth(t)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	a.RequireAuth(okHandler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location = %q, want /login", loc)
	}
}

func TestRequireAuthInvalidCookieRedirects(t *testing.T) {
	a := newTestAuth(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "bogus"})
	rec := httptest.NewRecorder()
	a.RequireAuth(okHandler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
}

func TestRequireAuthValidSession(t *testing.T) {
	a := newTestAuth(t)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	token, err := randomToken()
	if err != nil {
		t.Fatalf("randomToken: %v", err)
	}
	if err := a.store.CreateSession(context.Background(), userID, hashToken(token)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	var gotUser *store.AuthenticatedUser
	handler := a.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, _ = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotUser == nil || gotUser.Username != "rodolfo" || gotUser.Role != "vendedor" {
		t.Fatalf("context user = %+v", gotUser)
	}
}

func TestRequireAdmin(t *testing.T) {
	a := newTestAuth(t)
	vendedorID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")

	newSessionReq := func(userID int64) *http.Request {
		token, err := randomToken()
		if err != nil {
			t.Fatalf("randomToken: %v", err)
		}
		if err := a.store.CreateSession(context.Background(), userID, hashToken(token)); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		req := httptest.NewRequest("GET", "/usuarios", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		return req
	}

	handler := a.RequireAuth(a.RequireAdmin(okHandler()))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSessionReq(vendedorID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("vendedor status = %d, want 403", rec.Code)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, newSessionReq(adminID))
	if rec.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want 200", rec.Code)
	}
}

func TestLoginSubmitSuccess(t *testing.T) {
	a := newTestAuth(t)
	createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	form := url.Values{"username": {"rodolfo"}, "password": {"hunter2"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	a.LoginSubmit(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302; body = %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || cookies[0].Value == "" {
		t.Fatalf("cookies = %+v", cookies)
	}
}

func TestLoginSubmitWrongPassword(t *testing.T) {
	a := newTestAuth(t)
	createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	form := url.Values{"username": {"rodolfo"}, "password": {"wrong"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	a.LoginSubmit(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestLoginSubmitRateLimited(t *testing.T) {
	a := newTestAuth(t)
	createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	attempt := func() int {
		form := url.Values{"username": {"rodolfo"}, "password": {"wrong"}}
		req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "203.0.113.5:1234"
		rec := httptest.NewRecorder()
		a.LoginSubmit(rec, req)
		return rec.Code
	}

	for i := 1; i <= 5; i++ {
		if code := attempt(); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i, code)
		}
	}
	// The 6th attempt should be throttled before credentials are even checked.
	if code := attempt(); code != http.StatusTooManyRequests {
		t.Fatalf("6th attempt status = %d, want 429", code)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	a := newTestAuth(t)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")
	token, err := randomToken()
	if err != nil {
		t.Fatalf("randomToken: %v", err)
	}
	if err := a.store.CreateSession(context.Background(), userID, hashToken(token)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := httptest.NewRequest("POST", "/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	a.Logout(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location = %q, want /login", loc)
	}

	u, err := a.store.SessionUser(context.Background(), hashToken(token))
	if err != nil {
		t.Fatalf("SessionUser: %v", err)
	}
	if u != nil {
		t.Fatalf("session still valid after logout: %+v", u)
	}
}
