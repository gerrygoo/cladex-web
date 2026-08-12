package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestMiCuentaSubmitSuccess(t *testing.T) {
	a := newTestAuth(t)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	form := url.Values{
		"current_password": {"hunter2"},
		"new_password":     {"nuevopass123"},
		"confirm_password": {"nuevopass123"},
	}
	req := httptest.NewRequest("POST", "/mi-cuenta", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})

	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(a.MiCuentaSubmit)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Contraseña actualizada") {
		t.Fatalf("body missing success message: %s", rec.Body.String())
	}

	u, err := a.store.UserByUsername(context.Background(), "rodolfo")
	if err != nil {
		t.Fatalf("UserByUsername: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("nuevopass123")) != nil {
		t.Fatal("new password does not verify against stored hash")
	}
}

func mustSessionToken(t *testing.T, a *Auth, userID int64) string {
	t.Helper()
	token, err := randomToken()
	if err != nil {
		t.Fatalf("randomToken: %v", err)
	}
	if err := a.store.CreateSession(context.Background(), userID, hashToken(token)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return token
}

func TestMiCuentaSubmitWrongCurrentPassword(t *testing.T) {
	a := newTestAuth(t)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	form := url.Values{
		"current_password": {"wrong"},
		"new_password":     {"nuevopass123"},
		"confirm_password": {"nuevopass123"},
	}
	req := httptest.NewRequest("POST", "/mi-cuenta", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})

	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(a.MiCuentaSubmit)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}

	u, err := a.store.UserByUsername(context.Background(), "rodolfo")
	if err != nil {
		t.Fatalf("UserByUsername: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("hunter2")) != nil {
		t.Fatal("password was changed despite wrong current password")
	}
}

func TestMiCuentaSubmitMismatchedConfirmation(t *testing.T) {
	a := newTestAuth(t)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	form := url.Values{
		"current_password": {"hunter2"},
		"new_password":     {"nuevopass123"},
		"confirm_password": {"different123"},
	}
	req := httptest.NewRequest("POST", "/mi-cuenta", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})

	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(a.MiCuentaSubmit)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}
