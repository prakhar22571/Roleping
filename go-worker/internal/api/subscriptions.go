package api

import (
	"net/http"
	"strconv"

	"roleping-worker/internal/auth"
	"roleping-worker/internal/config"
	"roleping-worker/internal/db"
	"roleping-worker/internal/httprouter"
)

func subscriptionTarget(w http.ResponseWriter, r *http.Request) (userID, companyID int64, env *config.Env, ok bool) {
	companyID, err := strconv.ParseInt(httprouter.PathValue(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid company id")
		return 0, 0, nil, false
	}
	ident, found := auth.FromContext(r.Context())
	if !found {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return 0, 0, nil, false
	}
	env, err = config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return 0, 0, nil, false
	}
	company, err := db.GetCompany(r.Context(), env.DB, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return 0, 0, nil, false
	}
	if company == nil {
		writeError(w, http.StatusNotFound, "Company not found")
		return 0, 0, nil, false
	}
	return ident.UserID, companyID, env, true
}

func SubscribeHandler(w http.ResponseWriter, r *http.Request) {
	userID, companyID, env, ok := subscriptionTarget(w, r)
	if !ok {
		return
	}
	if err := db.SubscribeCompany(r.Context(), env.DB, userID, companyID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func UnsubscribeHandler(w http.ResponseWriter, r *http.Request) {
	userID, companyID, env, ok := subscriptionTarget(w, r)
	if !ok {
		return
	}
	if err := db.UnsubscribeCompany(r.Context(), env.DB, userID, companyID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
