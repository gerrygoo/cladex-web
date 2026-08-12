package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

const sessionCookieName = "cladex_session"

// dummyHash is compared against on every login attempt for a username that doesn't
// exist, so a failed login takes the same time whether or not the username is real —
// this defeats timing-based username enumeration. The password it was generated from
// is irrelevant and never used to authenticate anything.
const dummyHash = "$2a$12$u8XiYFJfZ3Ya.zqWtS2Rm.4uKi3dPwA51UNomgbauP3ZUFeo4BhdW"

// rateLimitThreshold is the number of prior failures (per username or per IP, since the
// last success) at which the next attempt starts being throttled.
const rateLimitThreshold = 5

// rateLimitCap is the maximum backoff delay, reached once failures pile up.
const rateLimitCap = 5 * time.Minute

// Auth holds the dependencies for authentication middleware and handlers.
type Auth struct {
	store        *store.Store
	cookieSecure bool
}

func NewAuth(s *store.Store, cookieSecure bool) *Auth {
	return &Auth{store: s, cookieSecure: cookieSecure}
}

type contextKey struct{ name string }

var userContextKey = &contextKey{"user"}

// UserFromContext returns the authenticated user attached by RequireAuth, if any.
func UserFromContext(ctx context.Context) (*store.AuthenticatedUser, bool) {
	u, ok := ctx.Value(userContextKey).(*store.AuthenticatedUser)
	return u, ok
}

// navUserView adapts a store.AuthenticatedUser to the trimmed view-model Layout needs,
// so internal/views doesn't have to import internal/store.
func navUserView(u *store.AuthenticatedUser) *views.NavUser {
	if u == nil {
		return nil
	}
	return &views.NavUser{Username: u.Username}
}

// RequireAuth redirects to /login unless the request carries a valid, unexpired
// session cookie for a non-disabled user. On success it renews the session (sliding
// expiry) and attaches the user to the request context.
func (a *Auth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		hash := hashToken(c.Value)
		user, err := a.store.SessionUser(r.Context(), hash)
		if err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
		if user == nil {
			a.clearSessionCookie(w)
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if err := a.store.RenewSession(r.Context(), hash); err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin gates a route to the "admin" role. It must run downstream of RequireAuth
// so a user is already in the request context; a vendedor gets a plain 403, not a
// redirect — this is enforcement, not just a hidden nav link.
func (a *Auth) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok || user.Role != "admin" {
			http.Error(w, "prohibido", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// LoginPage renders the login form.
func (a *Auth) LoginPage(w http.ResponseWriter, r *http.Request) {
	views.Login("", "").Render(r.Context(), w)
}

// LoginSubmit validates credentials, applies rate limiting, and on success starts a
// session and redirects to /.
func (a *Auth) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	ip := clientIP(r)
	ctx := r.Context()

	wait, err := a.rateLimitWait(ctx, username, ip)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if wait > 0 {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(wait.Seconds())+1))
		w.WriteHeader(http.StatusTooManyRequests)
		views.Login(fmt.Sprintf("Demasiados intentos. Intenta de nuevo en %d segundos.", int(wait.Seconds())+1), username).Render(ctx, w)
		return
	}

	user, err := a.store.UserByUsername(ctx, username)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	valid := false
	if user != nil && user.DisabledAt == nil {
		valid = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil
	} else {
		// Run bcrypt against a dummy hash so a nonexistent or disabled username takes
		// the same time as a real one with a wrong password.
		bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
	}

	if err := a.store.RecordLoginAttempt(ctx, username, ip, valid); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	if !valid {
		w.WriteHeader(http.StatusUnauthorized)
		views.Login("Usuario o contraseña incorrectos.", username).Render(ctx, w)
		return
	}

	token, err := randomToken()
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if err := a.store.CreateSession(ctx, user.ID, hashToken(token)); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	a.setSessionCookie(w, token)
	http.Redirect(w, r, "/", http.StatusFound)
}

// Logout deletes the session (if any) and clears the cookie.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = a.store.DeleteSession(r.Context(), hashToken(c.Value))
	}
	a.clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

// rateLimitWait returns how long the caller must wait before this username/IP pair may
// attempt another login, or 0 if they're not currently throttled. Failures are counted
// separately per username and per IP (since each one's last success), and the longer of
// the two waits applies — exponential backoff starting at the 6th consecutive failure.
func (a *Auth) rateLimitWait(ctx context.Context, username, ip string) (time.Duration, error) {
	uCount, uLast, err := a.store.UsernameFailureStreak(ctx, username)
	if err != nil {
		return 0, err
	}
	iCount, iLast, err := a.store.IPFailureStreak(ctx, ip)
	if err != nil {
		return 0, err
	}
	wait := backoffRemaining(uCount, uLast)
	if d := backoffRemaining(iCount, iLast); d > wait {
		wait = d
	}
	return wait, nil
}

func backoffRemaining(failures int, lastAttempt string) time.Duration {
	if failures < rateLimitThreshold || lastAttempt == "" {
		return 0
	}
	last, err := time.Parse(time.RFC3339Nano, lastAttempt)
	if err != nil {
		return 0
	}
	backoff := time.Duration(1) << uint(failures-rateLimitThreshold) * time.Second
	if backoff > rateLimitCap {
		backoff = rateLimitCap
	}
	elapsed := time.Since(last)
	if elapsed >= backoff {
		return 0
	}
	return backoff - elapsed
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (a *Auth) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   14 * 24 * 3600,
	})
}

func (a *Auth) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// clientIP extracts the caller's IP for rate limiting. The app sits behind nginx/FRP,
// which sets X-Forwarded-For; fall back to RemoteAddr for direct connections (local dev).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
