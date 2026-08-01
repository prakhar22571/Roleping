package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"roleping-worker/internal/auth"
	"roleping-worker/internal/config"
	"roleping-worker/internal/db"
	"roleping-worker/internal/httprouter"
)

type updateStatusBody struct {
	Status string `json:"status"`
}

func UpdateApplicationStatusHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(httprouter.PathValue(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	var body updateStatusBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !db.IsValidApplicationStatus(body.Status) {
		writeError(w, http.StatusBadRequest, "status must be one of: New, Applied, Interviewing, Rejected, Offer")
		return
	}

	env, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	job, err := db.GetJob(r.Context(), env.DB, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}

	ident, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := db.UpdateApplicationStatus(r.Context(), env.DB, id, ident.UserID, db.ApplicationStatus(body.Status)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"job_id": id, "status": body.Status})
}
