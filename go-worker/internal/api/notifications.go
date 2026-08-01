package api

import (
	"net/http"

	"roleping-worker/internal/config"
	"roleping-worker/internal/db"
)

func ListNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	env, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	notifications, err := db.ListNotifications(r.Context(), env.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, notifications)
}
