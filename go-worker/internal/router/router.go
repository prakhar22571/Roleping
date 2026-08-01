package router

import (
	"net/http"

	"roleping-worker/internal/api"
	"roleping-worker/internal/httprouter"
	"roleping-worker/internal/web"
)

func New() http.Handler {
	mux := httprouter.New()

	// JSON API
	mux.Handle("GET /api/me", api.MeHandler)

	mux.Handle("GET /api/companies", api.ListCompaniesHandler)
	mux.Handle("POST /api/companies", api.CreateCompanyHandler)
	mux.Handle("PUT /api/companies/{id}", api.UpdateCompanyHandler)
	mux.Handle("DELETE /api/companies/{id}", api.DeactivateCompanyHandler)
	mux.Handle("POST /api/companies/{id}/subscription", api.SubscribeHandler)
	mux.Handle("DELETE /api/companies/{id}/subscription", api.UnsubscribeHandler)

	mux.Handle("GET /api/jobs", api.ListJobsHandler)
	mux.Handle("GET /api/jobs/{id}", api.GetJobDetailHandler)
	mux.Handle("GET /api/jobs/{id}/verdicts", api.ListVerdictsHandler)
	mux.Handle("PATCH /api/jobs/{id}/status", api.UpdateApplicationStatusHandler)

	mux.Handle("GET /api/notifications", api.ListNotificationsHandler)
	mux.Handle("POST /api/run-now", api.RunNowHandler)

	// Server-rendered dashboard
	web.RegisterRoutes(mux)

	return mux
}
