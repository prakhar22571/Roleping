package api

import (
	"net/http"

	"roleping-worker/internal/auth"
	"roleping-worker/internal/config"
	"roleping-worker/internal/db"
)

func ListNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	ident, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	env, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	notifications, err := db.ListNotificationsForUser(r.Context(), env.DB, ident.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, notifications)
}
