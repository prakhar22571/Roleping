package api

import (
	"net/http"
	"strconv"

	"roleping-worker/internal/auth"
	"roleping-worker/internal/config"
	"roleping-worker/internal/db"
	"roleping-worker/internal/httprouter"
)

type jobDetailResponse struct {
	db.JobListRow
	Verdicts     []db.LlmVerdict  `json:"verdicts"`
	Notification *db.Notification `json:"notification"`
}

func ListJobsHandler(w http.ResponseWriter, r *http.Request) {
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

	q := r.URL.Query()
	filters := db.JobListFilters{}

	if companyIDStr := q.Get("companyId"); companyIDStr != "" {
		if companyID, err := strconv.ParseInt(companyIDStr, 10, 64); err == nil {
			filters.CompanyID = &companyID
		}
	}
	if statusStr := q.Get("status"); statusStr != "" && db.IsValidApplicationStatus(statusStr) {
		status := db.ApplicationStatus(statusStr)
		filters.Status = &status
	}
	filters.DisagreementOnly = q.Get("disagreement") == "true"
	filters.SubscribedOnly = q.Get("subscribed") == "true"
	if limitStr := q.Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			filters.Limit = limit
		}
	}
	if offsetStr := q.Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			filters.Offset = offset
		}
	}

	jobs, err := db.ListJobs(r.Context(), env.DB, ident.UserID, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

func GetJobDetailHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(httprouter.PathValue(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}

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

	jobRow, err := db.GetJobListRow(r.Context(), env.DB, id, ident.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if jobRow == nil {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}

	verdicts, err := db.ListVerdictsForJob(r.Context(), env.DB, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	notification, err := db.GetNotificationForJob(r.Context(), env.DB, id, ident.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, jobDetailResponse{
		JobListRow:   *jobRow,
		Verdicts:     verdicts,
		Notification: notification,
	})
}
