package api

import (
	"net/http"

	"roleping-worker/internal/auth"
)

func MeHandler(w http.ResponseWriter, r *http.Request) {
	ident, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":  ident.UserID,
		"email":    ident.Email,
		"is_owner": ident.IsOwner,
	})
}
