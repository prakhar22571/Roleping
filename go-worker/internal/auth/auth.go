// Package auth resolves the authenticated user for every HTTP request.
//
// Authentication itself happens at the Cloudflare edge: a Cloudflare Access
// application in front of the worker rejects unauthenticated traffic and
// stamps Cf-Access-Authenticated-User-Email on requests that pass. This
// package trusts that header, auto-provisions a users row on first sight,
// and exposes the identity via the request context.
//
// If the Access application is removed, this header becomes spoofable and
// the app is effectively public - see RUNBOOK.md.
package auth

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/syumai/workers/cloudflare"

	"roleping-worker/internal/db"
)

// accessEmailHeader is set by Cloudflare Access on authenticated requests.
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

func getenv(name string) string {
	v := cloudflare.Getenv(name)
	if v == jsNullString {
		return ""
	}
	return v
}

// Middleware resolves the request's identity and stores it in the context.
// The Access header wins; DEV_USER_EMAIL (set only in .dev.vars, never in
// wrangler.jsonc) is the local-dev fallback so `wrangler dev` works without
// an Access gate. No identity means 401.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email := r.Header.Get(accessEmailHeader)
		if email == "" {
			email = getenv("DEV_USER_EMAIL")
		}
		if email == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		conn, err := sql.Open("d1", "DB")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		user, err := db.GetOrCreateUser(r.Context(), conn, email)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		ident := Identity{
			UserID:  user.ID,
			Email:   user.Email,
			IsOwner: strings.EqualFold(user.Email, getenv("OWNER_EMAIL")),
		}
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), ident)))
	})
}
