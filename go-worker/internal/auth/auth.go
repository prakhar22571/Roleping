// Package auth resolves the authenticated user for every HTTP request.
//
// Identity comes from a signed session cookie issued by the login form (see
// session.go). The cookie carries the user id, the user's session epoch, and
// an expiry, all covered by an HMAC keyed with the SESSION_SECRET binding, so
// a client can't forge or edit one.
//
// A note on Cloudflare Access: an earlier version of this app was gated by
// Access at the edge and simply trusted the Cf-Access-Authenticated-User-Email
// header it stamps on requests. That is only safe while Access actually fronts
// every request - without it, anyone can send that header and become any user.
// The header is therefore ignored unless TRUST_ACCESS_HEADER is explicitly set
// to "true", which should happen only if an Access application is put back in
// front of the Worker.
package auth

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/syumai/workers/cloudflare"

	"roleping-worker/internal/db"
)

const accessEmailHeader = "Cf-Access-Authenticated-User-Email"

// jsNullString is what cloudflare.Getenv yields for an unset var under
// syscall/js (same quirk as config.jsNullString).
const jsNullString = "<null>"

type Identity struct {
	UserID  int64
	Email   string
	IsOwner bool
}

type contextKey struct{}

func WithIdentity(ctx context.Context, ident Identity) context.Context {
	return context.WithValue(ctx, contextKey{}, ident)
}

func FromContext(ctx context.Context) (Identity, bool) {
	ident, ok := ctx.Value(contextKey{}).(Identity)
	return ident, ok
}

func Getenv(name string) string {
	v := cloudflare.Getenv(name)
	if v == jsNullString {
		return ""
	}
	return v
}

// SessionSecret is the HMAC key for session cookies. Falling back to a
// constant when unset would silently make every cookie forgeable, so an
// unset secret is treated as fatal by the callers instead.
func SessionSecret() []byte {
	return []byte(Getenv("SESSION_SECRET"))
}

func OwnerEmail() string {
	return Getenv("OWNER_EMAIL")
}

func isOwnerEmail(email string) bool {
	owner := OwnerEmail()
	return owner != "" && strings.EqualFold(email, owner)
}

// publicPaths are reachable without a session - otherwise nobody could ever
// log in.
func isPublicPath(path string) bool {
	return path == "/login" || path == "/logout" || path == "/healthz"
}

// resolve determines who is making this request, or nil for nobody.
func resolve(r *http.Request, conn *sql.DB) (*db.User, error) {
	// Access header, only when an Access application is known to be in front.
	if Getenv("TRUST_ACCESS_HEADER") == "true" {
		if email := r.Header.Get(accessEmailHeader); email != "" {
			return db.GetOrCreateUser(r.Context(), conn, email)
		}
	}

	// Session cookie: the normal path.
	if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
		secret := SessionSecret()
		if len(secret) == 0 {
			return nil, nil
		}
		sess, err := parseSession(secret, cookie.Value)
		if err != nil {
			return nil, nil
		}
		user, err := db.GetUserByID(r.Context(), conn, sess.UserID)
		if err != nil {
			return nil, err
		}
		// A password change bumps session_epoch, retiring older cookies.
		if user == nil || user.SessionEpoch != sess.Epoch {
			return nil, nil
		}
		return user, nil
	}

	// Local dev convenience: `wrangler dev` has no login unless you want one.
	// DEV_USER_EMAIL lives only in .dev.vars and must never be set in
	// production, where its presence would hand out a free identity.
	if email := Getenv("DEV_USER_EMAIL"); email != "" {
		return db.GetOrCreateUser(r.Context(), conn, email)
	}

	return nil, nil
}

// Middleware resolves the request's identity and stores it in the context,
// bouncing anonymous requests to the login page (or a 401 for API calls).
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := sql.Open("d1", "DB")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		user, err := resolve(r, conn)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		ctx := r.Context()
		if user != nil {
			ctx = WithIdentity(ctx, Identity{
				UserID:  user.ID,
				Email:   user.Email,
				IsOwner: isOwnerEmail(user.Email),
			})
		}

		if user == nil && !isPublicPath(r.URL.Path) {
			// API clients want a status code; browsers want the login form.
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
