package api

import (
	"net/http"

	"roleping-worker/internal/adapters"
	"roleping-worker/internal/config"
	"roleping-worker/internal/jobs"
)

func RunNowHandler(w http.ResponseWriter, r *http.Request) {
	env, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	client := config.NewFetchClient()
	registry := adapters.NewRegistry(client)

	summary, err := jobs.RunPipeline(r.Context(), env, registry, client)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
