package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/a-h/templ"

	"roleping-worker/internal/adapters"
	"roleping-worker/internal/auth"
	"roleping-worker/internal/config"
	"roleping-worker/internal/db"
	"roleping-worker/internal/httprouter"
	"roleping-worker/internal/jobs"
)

// identity fetches the authenticated identity set by auth.Middleware,
// writing a 401 if it is somehow missing.
func identity(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	ident, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
	return ident, ok
}

func RegisterRoutes(mux *httprouter.Router) {
	mux.Handle("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/jobs", http.StatusFound)
	})

	mux.Handle("GET /jobs", jobsPageHandler)
	mux.Handle("GET /jobs/table", jobsTableHandler)
	mux.Handle("GET /jobs/{id}", jobDetailPageHandler)
	mux.Handle("POST /jobs/{id}/status", updateStatusHandler)

	mux.Handle("GET /companies", companiesPageHandler)
	mux.Handle("POST /companies", createCompanyHandler)
	mux.Handle("POST /companies/{id}/deactivate", deactivateCompanyHandler)
	mux.Handle("POST /companies/{id}/subscribe", subscribeCompanyHandler)
	mux.Handle("POST /companies/{id}/unsubscribe", unsubscribeCompanyHandler)

	mux.Handle("GET /notifications", notificationsPageHandler)

	mux.Handle("POST /run-now", runNowHandler)
}

func jobFiltersFromQuery(r *http.Request) db.JobListFilters {
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
	if search := q.Get("search"); search != "" {
		filters.TitleContains = &search
	}
	return filters
}

func jobsPageHandler(w http.ResponseWriter, r *http.Request) {
	ident, ok := identity(w, r)
	if !ok {
		return
	}

	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jobRows, err := db.ListJobs(r.Context(), env.DB, ident.UserID, db.JobListFilters{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	companies, err := db.ListCompanies(r.Context(), env.DB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templ.Handler(JobsPage(jobRows, companies, ident.Email, ident.IsOwner)).ServeHTTP(w, r)
}

func jobsTableHandler(w http.ResponseWriter, r *http.Request) {
	ident, ok := identity(w, r)
	if !ok {
		return
	}

	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jobRows, err := db.ListJobs(r.Context(), env.DB, ident.UserID, jobFiltersFromQuery(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templ.Handler(JobsTable(jobRows)).ServeHTTP(w, r)
}

func jobDetailPageHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(httprouter.PathValue(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid job id", http.StatusBadRequest)
		return
	}

	ident, ok := identity(w, r)
	if !ok {
		return
	}

	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jobRow, err := db.GetJobListRow(r.Context(), env.DB, id, ident.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if jobRow == nil {
		http.NotFound(w, r)
		return
	}

	verdicts, err := db.ListVerdictsForJob(r.Context(), env.DB, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	notification, err := db.GetNotificationForJob(r.Context(), env.DB, id, ident.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templ.Handler(JobDetailPage(*jobRow, verdicts, notification, ident.Email, ident.IsOwner)).ServeHTTP(w, r)
}

func updateStatusHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(httprouter.PathValue(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid job id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form body", http.StatusBadRequest)
		return
	}
	status := r.FormValue("status")
	if !db.IsValidApplicationStatus(status) {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	ident, ok := identity(w, r)
	if !ok {
		return
	}

	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := db.UpdateApplicationStatus(r.Context(), env.DB, id, ident.UserID, db.ApplicationStatus(status)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templ.Handler(StatusEditor(id, db.ApplicationStatus(status))).ServeHTTP(w, r)
}

func companiesPageHandler(w http.ResponseWriter, r *http.Request) {
	ident, ok := identity(w, r)
	if !ok {
		return
	}

	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	companies, err := db.ListCompaniesWithSubscription(r.Context(), env.DB, ident.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templ.Handler(CompaniesPage(companies, ident.Email, ident.IsOwner)).ServeHTTP(w, r)
}

// renderCompaniesTable re-renders the htmx companies table fragment for the
// current user.
func renderCompaniesTable(w http.ResponseWriter, r *http.Request, env *config.Env, userID int64) {
	companies, err := db.ListCompaniesWithSubscription(r.Context(), env.DB, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	templ.Handler(CompaniesTable(companies)).ServeHTTP(w, r)
}

func subscribeCompanyHandler(w http.ResponseWriter, r *http.Request) {
	toggleSubscriptionHandler(w, r, db.SubscribeCompany)
}

func unsubscribeCompanyHandler(w http.ResponseWriter, r *http.Request) {
	toggleSubscriptionHandler(w, r, db.UnsubscribeCompany)
}

func toggleSubscriptionHandler(w http.ResponseWriter, r *http.Request, apply func(ctx context.Context, conn *sql.DB, userID, companyID int64) error) {
	id, err := strconv.ParseInt(httprouter.PathValue(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid company id", http.StatusBadRequest)
		return
	}

	ident, ok := identity(w, r)
	if !ok {
		return
	}

	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := apply(r.Context(), env.DB, ident.UserID, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	renderCompaniesTable(w, r, env, ident.UserID)
}

func notificationsPageHandler(w http.ResponseWriter, r *http.Request) {
	ident, ok := identity(w, r)
	if !ok {
		return
	}

	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	notifications, err := db.ListNotificationsForUser(r.Context(), env.DB, ident.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templ.Handler(NotificationsPage(notifications, ident.Email, ident.IsOwner)).ServeHTTP(w, r)
}

func buildAdapterConfig(adapterType, boardToken, leverCompany, resultLimitStr string) *string {
	var raw map[string]any
	switch adapterType {
	case string(adapters.Greenhouse):
		raw = map[string]any{"boardToken": boardToken}
	case string(adapters.Lever):
		raw = map[string]any{"company": leverCompany}
	case string(adapters.Amazon):
		limit := 20
		if v, err := strconv.Atoi(resultLimitStr); err == nil && v > 0 {
			limit = v
		}
		raw = map[string]any{"resultLimit": limit}
	default:
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

func createCompanyHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form body", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	portalURL := r.FormValue("portal_url")
	adapterType := r.FormValue("adapter_type")
	if name == "" || portalURL == "" || !db.IsValidAdapterType(adapterType) {
		http.Error(w, "name, portal_url, and a valid adapter_type are required", http.StatusBadRequest)
		return
	}

	ident, ok := identity(w, r)
	if !ok {
		return
	}

	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	adapterConfig := buildAdapterConfig(adapterType, r.FormValue("board_token"), r.FormValue("lever_company"), r.FormValue("result_limit"))

	company, err := db.CreateCompany(r.Context(), env.DB, db.CreateCompanyInput{
		Name:          name,
		PortalURL:     portalURL,
		AdapterType:   adapters.AdapterType(adapterType),
		AdapterConfig: adapterConfig,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Whoever adds a company almost certainly wants alerts for it.
	if err := db.SubscribeCompany(r.Context(), env.DB, ident.UserID, company.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	renderCompaniesTable(w, r, env, ident.UserID)
}

func deactivateCompanyHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(httprouter.PathValue(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid company id", http.StatusBadRequest)
		return
	}

	ident, ok := identity(w, r)
	if !ok {
		return
	}

	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	isActive := false
	if _, err := db.UpdateCompany(r.Context(), env.DB, id, db.UpdateCompanyInput{IsActive: &isActive}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	renderCompaniesTable(w, r, env, ident.UserID)
}

func runNowHandler(w http.ResponseWriter, r *http.Request) {
	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	client := config.NewFetchClient()
	registry := adapters.NewRegistry(client)

	summary, err := jobs.RunPipeline(r.Context(), env, registry, client)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "Processed %d companies (%d failed), found %d new jobs, sent %d emails.",
		summary.CompaniesProcessed, summary.CompaniesFailed, summary.NewJobs, summary.NotificationsSent)
}
