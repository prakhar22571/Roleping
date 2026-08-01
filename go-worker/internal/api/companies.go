package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"roleping-worker/internal/adapters"
	"roleping-worker/internal/config"
	"roleping-worker/internal/db"
	"roleping-worker/internal/httprouter"
)

type createCompanyBody struct {
	Name          string          `json:"name"`
	PortalURL     string          `json:"portal_url"`
	AdapterType   string          `json:"adapter_type"`
	AdapterConfig json.RawMessage `json:"adapter_config"`
}

func ListCompaniesHandler(w http.ResponseWriter, r *http.Request) {
	env, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	companies, err := db.ListCompanies(r.Context(), env.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, companies)
}

func CreateCompanyHandler(w http.ResponseWriter, r *http.Request) {
	var body createCompanyBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Name == "" || body.PortalURL == "" || body.AdapterType == "" {
		writeError(w, http.StatusBadRequest, "name, portal_url, and adapter_type are required")
		return
	}
	if !db.IsValidAdapterType(body.AdapterType) {
		writeError(w, http.StatusBadRequest, "adapter_type must be one of: amazon, greenhouse, lever")
		return
	}

	env, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var adapterConfig *string
	if len(body.AdapterConfig) > 0 {
		s := string(body.AdapterConfig)
		adapterConfig = &s
	}

	company, err := db.CreateCompany(r.Context(), env.DB, db.CreateCompanyInput{
		Name:          body.Name,
		PortalURL:     body.PortalURL,
		AdapterType:   adapters.AdapterType(body.AdapterType),
		AdapterConfig: adapterConfig,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, company)
}

type updateCompanyBody struct {
	Name          *string         `json:"name"`
	PortalURL     *string         `json:"portal_url"`
	AdapterType   *string         `json:"adapter_type"`
	AdapterConfig json.RawMessage `json:"adapter_config"`
	IsActive      *bool           `json:"is_active"`
}

func UpdateCompanyHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(httprouter.PathValue(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid company id")
		return
	}

	var body updateCompanyBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.AdapterType != nil && !db.IsValidAdapterType(*body.AdapterType) {
		writeError(w, http.StatusBadRequest, "adapter_type must be one of: amazon, greenhouse, lever")
		return
	}

	env, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	input := db.UpdateCompanyInput{
		Name:      body.Name,
		PortalURL: body.PortalURL,
		IsActive:  body.IsActive,
	}
	if body.AdapterType != nil {
		at := adapters.AdapterType(*body.AdapterType)
		input.AdapterType = &at
	}
	if len(body.AdapterConfig) > 0 {
		s := string(body.AdapterConfig)
		sp := &s
		input.AdapterConfig = &sp
	}

	company, err := db.UpdateCompany(r.Context(), env.DB, id, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if company == nil {
		writeError(w, http.StatusNotFound, "Company not found")
		return
	}
	writeJSON(w, http.StatusOK, company)
}

func DeactivateCompanyHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(httprouter.PathValue(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid company id")
		return
	}

	env, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	isActive := false
	company, err := db.UpdateCompany(r.Context(), env.DB, id, db.UpdateCompanyInput{IsActive: &isActive})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if company == nil {
		writeError(w, http.StatusNotFound, "Company not found")
		return
	}
	writeJSON(w, http.StatusOK, company)
}
