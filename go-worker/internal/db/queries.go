package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"roleping-worker/internal/adapters"
)

func nullableString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func scanNullString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}

func ListCompanies(ctx context.Context, conn *sql.DB) ([]Company, error) {
	rows, err := conn.QueryContext(ctx, "SELECT id, name, portal_url, adapter_type, adapter_config, is_active, created_at, updated_at FROM companies ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var companies []Company
	for rows.Next() {
		var c Company
		var adapterType string
		var adapterConfig sql.NullString
		var isActive int
		if err := rows.Scan(&c.ID, &c.Name, &c.PortalURL, &adapterType, &adapterConfig, &isActive, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.AdapterType = adapters.AdapterType(adapterType)
		c.AdapterConfig = scanNullString(adapterConfig)
		c.IsActive = isActive != 0
		companies = append(companies, c)
	}
	return companies, rows.Err()
}

func ListActiveCompanies(ctx context.Context, conn *sql.DB) ([]Company, error) {
	rows, err := conn.QueryContext(ctx, "SELECT id, name, portal_url, adapter_type, adapter_config, is_active, created_at, updated_at FROM companies WHERE is_active = 1 ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var companies []Company
	for rows.Next() {
		var c Company
		var adapterType string
		var adapterConfig sql.NullString
		var isActive int
		if err := rows.Scan(&c.ID, &c.Name, &c.PortalURL, &adapterType, &adapterConfig, &isActive, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.AdapterType = adapters.AdapterType(adapterType)
		c.AdapterConfig = scanNullString(adapterConfig)
		c.IsActive = isActive != 0
		companies = append(companies, c)
	}
	return companies, rows.Err()
}

func GetCompany(ctx context.Context, conn *sql.DB, id int64) (*Company, error) {
	var c Company
	var adapterType string
	var adapterConfig sql.NullString
	var isActive int
	err := conn.QueryRowContext(ctx, "SELECT id, name, portal_url, adapter_type, adapter_config, is_active, created_at, updated_at FROM companies WHERE id = ?", id).
		Scan(&c.ID, &c.Name, &c.PortalURL, &adapterType, &adapterConfig, &isActive, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.AdapterType = adapters.AdapterType(adapterType)
	c.AdapterConfig = scanNullString(adapterConfig)
	c.IsActive = isActive != 0
	return &c, nil
}

type CreateCompanyInput struct {
	Name          string
	PortalURL     string
	AdapterType   adapters.AdapterType
	AdapterConfig *string
}

func CreateCompany(ctx context.Context, conn *sql.DB, input CreateCompanyInput) (*Company, error) {
	result, err := conn.ExecContext(ctx,
		"INSERT INTO companies (name, portal_url, adapter_type, adapter_config) VALUES (?, ?, ?, ?)",
		input.Name, input.PortalURL, string(input.AdapterType), nullableString(input.AdapterConfig),
	)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return GetCompany(ctx, conn, id)
}

type UpdateCompanyInput struct {
	Name          *string
	PortalURL     *string
	AdapterType   *adapters.AdapterType
	AdapterConfig **string // nil = leave unchanged, non-nil pointing to nil = clear the value
	IsActive      *bool
}

func UpdateCompany(ctx context.Context, conn *sql.DB, id int64, input UpdateCompanyInput) (*Company, error) {
	existing, err := GetCompany(ctx, conn, id)
	if err != nil || existing == nil {
		return nil, err
	}

	name := existing.Name
	if input.Name != nil {
		name = *input.Name
	}
	portalURL := existing.PortalURL
	if input.PortalURL != nil {
		portalURL = *input.PortalURL
	}
	adapterType := existing.AdapterType
	if input.AdapterType != nil {
		adapterType = *input.AdapterType
	}
	adapterConfig := existing.AdapterConfig
	if input.AdapterConfig != nil {
		adapterConfig = *input.AdapterConfig
	}
	isActive := existing.IsActive
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	isActiveInt := 0
	if isActive {
		isActiveInt = 1
	}

	_, err = conn.ExecContext(ctx,
		`UPDATE companies SET name = ?, portal_url = ?, adapter_type = ?, adapter_config = ?, is_active = ?, updated_at = datetime('now') WHERE id = ?`,
		name, portalURL, string(adapterType), nullableString(adapterConfig), isActiveInt, id,
	)
	if err != nil {
		return nil, err
	}
	return GetCompany(ctx, conn, id)
}

func FindJobBySourceExternalID(ctx context.Context, conn *sql.DB, source adapters.AdapterType, externalID string) (*Job, error) {
	return scanJobRow(conn.QueryRowContext(ctx,
		"SELECT id, external_id, source, company_id, title, location, posted_date, qualifications_text, apply_url, raw_json, first_seen_at FROM jobs WHERE source = ? AND external_id = ?",
		string(source), externalID,
	))
}

func GetJob(ctx context.Context, conn *sql.DB, id int64) (*Job, error) {
	return scanJobRow(conn.QueryRowContext(ctx,
		"SELECT id, external_id, source, company_id, title, location, posted_date, qualifications_text, apply_url, raw_json, first_seen_at FROM jobs WHERE id = ?",
		id,
	))
}

func scanJobRow(row *sql.Row) (*Job, error) {
	var j Job
	var source string
	var location, postedDate, qualificationsText, rawJSON sql.NullString
	err := row.Scan(&j.ID, &j.ExternalID, &source, &j.CompanyID, &j.Title, &location, &postedDate, &qualificationsText, &j.ApplyURL, &rawJSON, &j.FirstSeenAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	j.Source = adapters.AdapterType(source)
	j.Location = scanNullString(location)
	j.PostedDate = scanNullString(postedDate)
	j.QualificationsText = scanNullString(qualificationsText)
	j.RawJSON = scanNullString(rawJSON)
	return &j, nil
}

func InsertJob(ctx context.Context, conn *sql.DB, companyID int64, source adapters.AdapterType, job adapters.NormalizedJob) (*Job, error) {
	var rawJSON *string
	if len(job.Raw) > 0 {
		s := string(job.Raw)
		rawJSON = &s
	}

	result, err := conn.ExecContext(ctx,
		`INSERT INTO jobs (external_id, source, company_id, title, location, posted_date, qualifications_text, apply_url, raw_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ExternalID, string(source), companyID, job.Title, nullableString(job.Location), nullableString(job.PostedDate), job.QualificationsText, job.ApplyURL, nullableString(rawJSON),
	)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	if _, err := conn.ExecContext(ctx, "INSERT OR IGNORE INTO application_status (job_id, status) VALUES (?, 'New')", id); err != nil {
		return nil, err
	}

	return GetJob(ctx, conn, id)
}

type VerdictInput struct {
	JobID       int64
	ModelID     string
	Match       bool
	Confidence  *float64
	Reasoning   string
	RawResponse *string
}

func UpsertVerdict(ctx context.Context, conn *sql.DB, v VerdictInput) error {
	matchInt := 0
	if v.Match {
		matchInt = 1
	}
	var confidence any
	if v.Confidence != nil {
		confidence = *v.Confidence
	}

	_, err := conn.ExecContext(ctx,
		`INSERT INTO llm_verdicts (job_id, model_id, match, confidence, reasoning, raw_response)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (job_id, model_id) DO UPDATE SET
		   match = excluded.match,
		   confidence = excluded.confidence,
		   reasoning = excluded.reasoning,
		   raw_response = excluded.raw_response,
		   classified_at = datetime('now')`,
		v.JobID, v.ModelID, matchInt, confidence, v.Reasoning, nullableString(v.RawResponse),
	)
	return err
}

func ListVerdictsForJob(ctx context.Context, conn *sql.DB, jobID int64) ([]LlmVerdict, error) {
	rows, err := conn.QueryContext(ctx,
		"SELECT id, job_id, model_id, match, confidence, reasoning, raw_response, classified_at FROM llm_verdicts WHERE job_id = ? ORDER BY classified_at",
		jobID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var verdicts []LlmVerdict
	for rows.Next() {
		var v LlmVerdict
		var match int
		var confidence sql.NullFloat64
		var rawResponse sql.NullString
		if err := rows.Scan(&v.ID, &v.JobID, &v.ModelID, &match, &confidence, &v.Reasoning, &rawResponse, &v.ClassifiedAt); err != nil {
			return nil, err
		}
		v.Match = match != 0
		if confidence.Valid {
			c := confidence.Float64
			v.Confidence = &c
		}
		v.RawResponse = scanNullString(rawResponse)
		verdicts = append(verdicts, v)
	}
	return verdicts, rows.Err()
}

type NotificationInput struct {
	JobID       int64
	EmailStatus EmailStatus
	ResendID    *string
	ErrorDetail *string
}

func RecordNotification(ctx context.Context, conn *sql.DB, input NotificationInput) error {
	_, err := conn.ExecContext(ctx,
		`INSERT INTO notifications (job_id, email_status, resend_id, error_detail)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (job_id) DO UPDATE SET
		   email_status = excluded.email_status,
		   resend_id = excluded.resend_id,
		   error_detail = excluded.error_detail,
		   sent_at = datetime('now')`,
		input.JobID, string(input.EmailStatus), nullableString(input.ResendID), nullableString(input.ErrorDetail),
	)
	return err
}

func GetNotificationForJob(ctx context.Context, conn *sql.DB, jobID int64) (*Notification, error) {
	var n Notification
	var emailStatus string
	var resendID, errorDetail sql.NullString
	err := conn.QueryRowContext(ctx,
		"SELECT id, job_id, sent_at, email_status, resend_id, error_detail FROM notifications WHERE job_id = ?", jobID,
	).Scan(&n.ID, &n.JobID, &n.SentAt, &emailStatus, &resendID, &errorDetail)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	n.EmailStatus = EmailStatus(emailStatus)
	n.ResendID = scanNullString(resendID)
	n.ErrorDetail = scanNullString(errorDetail)
	return &n, nil
}

func ListNotifications(ctx context.Context, conn *sql.DB) ([]Notification, error) {
	rows, err := conn.QueryContext(ctx, "SELECT id, job_id, sent_at, email_status, resend_id, error_detail FROM notifications ORDER BY sent_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []Notification
	for rows.Next() {
		var n Notification
		var emailStatus string
		var resendID, errorDetail sql.NullString
		if err := rows.Scan(&n.ID, &n.JobID, &n.SentAt, &emailStatus, &resendID, &errorDetail); err != nil {
			return nil, err
		}
		n.EmailStatus = EmailStatus(emailStatus)
		n.ResendID = scanNullString(resendID)
		n.ErrorDetail = scanNullString(errorDetail)
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

func UpdateApplicationStatus(ctx context.Context, conn *sql.DB, jobID int64, status ApplicationStatus) error {
	_, err := conn.ExecContext(ctx,
		`INSERT INTO application_status (job_id, status) VALUES (?, ?)
		 ON CONFLICT (job_id) DO UPDATE SET status = excluded.status, updated_at = datetime('now')`,
		jobID, string(status),
	)
	return err
}

type JobListFilters struct {
	CompanyID        *int64
	Status           *ApplicationStatus
	DisagreementOnly bool
	TitleContains    *string
	Limit            int
	Offset           int
}

type JobListRow struct {
	Job
	CompanyName       string            `json:"company_name"`
	ApplicationStatus ApplicationStatus `json:"application_status"`
	Notified          bool              `json:"notified"`
	VerdictsJSON      *string           `json:"verdicts_json"`
}

func ListJobs(ctx context.Context, conn *sql.DB, filters JobListFilters) ([]JobListRow, error) {
	var conditions []string
	var args []any

	if filters.CompanyID != nil {
		conditions = append(conditions, "jobs.company_id = ?")
		args = append(args, *filters.CompanyID)
	}
	if filters.Status != nil {
		conditions = append(conditions, "application_status.status = ?")
		args = append(args, string(*filters.Status))
	}
	if filters.DisagreementOnly {
		conditions = append(conditions, "(SELECT COUNT(DISTINCT match) FROM llm_verdicts WHERE llm_verdicts.job_id = jobs.id) > 1")
	}
	if filters.TitleContains != nil && *filters.TitleContains != "" {
		conditions = append(conditions, "jobs.title LIKE ?")
		args = append(args, "%"+*filters.TitleContains+"%")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}
	args = append(args, limit, filters.Offset)

	query := fmt.Sprintf(`
		SELECT jobs.id, jobs.external_id, jobs.source, jobs.company_id, jobs.title, jobs.location,
		       jobs.posted_date, jobs.qualifications_text, jobs.apply_url, jobs.raw_json, jobs.first_seen_at,
		       companies.name AS company_name,
		       application_status.status AS application_status,
		       CASE WHEN notifications.email_status = 'sent' THEN 1 ELSE 0 END AS notified,
		       (SELECT json_group_array(json_object('model_id', model_id, 'match', match, 'reasoning', reasoning))
		        FROM llm_verdicts WHERE llm_verdicts.job_id = jobs.id) AS verdicts_json
		FROM jobs
		JOIN companies ON companies.id = jobs.company_id
		LEFT JOIN application_status ON application_status.job_id = jobs.id
		LEFT JOIN notifications ON notifications.job_id = jobs.id
		%s
		ORDER BY jobs.first_seen_at DESC
		LIMIT ? OFFSET ?`, whereClause)

	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []JobListRow
	for rows.Next() {
		var r JobListRow
		var source string
		var location, postedDate, qualificationsText, rawJSON, verdictsJSON sql.NullString
		var applicationStatus sql.NullString
		var notified int
		if err := rows.Scan(
			&r.ID, &r.ExternalID, &source, &r.CompanyID, &r.Title, &location,
			&postedDate, &qualificationsText, &r.ApplyURL, &rawJSON, &r.FirstSeenAt,
			&r.CompanyName, &applicationStatus, &notified, &verdictsJSON,
		); err != nil {
			return nil, err
		}
		r.Source = adapters.AdapterType(source)
		r.Location = scanNullString(location)
		r.PostedDate = scanNullString(postedDate)
		r.QualificationsText = scanNullString(qualificationsText)
		r.RawJSON = scanNullString(rawJSON)
		if applicationStatus.Valid {
			r.ApplicationStatus = ApplicationStatus(applicationStatus.String)
		} else {
			r.ApplicationStatus = StatusNew
		}
		r.Notified = notified != 0
		r.VerdictsJSON = scanNullString(verdictsJSON)
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetJobListRow fetches a single job with its company name, application
// status, notification status, and verdict summary joined in, for detail views.
func GetJobListRow(ctx context.Context, conn *sql.DB, id int64) (*JobListRow, error) {
	row := conn.QueryRowContext(ctx, `
		SELECT jobs.id, jobs.external_id, jobs.source, jobs.company_id, jobs.title, jobs.location,
		       jobs.posted_date, jobs.qualifications_text, jobs.apply_url, jobs.raw_json, jobs.first_seen_at,
		       companies.name AS company_name,
		       application_status.status AS application_status,
		       CASE WHEN notifications.email_status = 'sent' THEN 1 ELSE 0 END AS notified,
		       (SELECT json_group_array(json_object('model_id', model_id, 'match', match, 'reasoning', reasoning))
		        FROM llm_verdicts WHERE llm_verdicts.job_id = jobs.id) AS verdicts_json
		FROM jobs
		JOIN companies ON companies.id = jobs.company_id
		LEFT JOIN application_status ON application_status.job_id = jobs.id
		LEFT JOIN notifications ON notifications.job_id = jobs.id
		WHERE jobs.id = ?`, id)

	var r JobListRow
	var source string
	var location, postedDate, qualificationsText, rawJSON, verdictsJSON sql.NullString
	var applicationStatus sql.NullString
	var notified int
	err := row.Scan(
		&r.ID, &r.ExternalID, &source, &r.CompanyID, &r.Title, &location,
		&postedDate, &qualificationsText, &r.ApplyURL, &rawJSON, &r.FirstSeenAt,
		&r.CompanyName, &applicationStatus, &notified, &verdictsJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Source = adapters.AdapterType(source)
	r.Location = scanNullString(location)
	r.PostedDate = scanNullString(postedDate)
	r.QualificationsText = scanNullString(qualificationsText)
	r.RawJSON = scanNullString(rawJSON)
	if applicationStatus.Valid {
		r.ApplicationStatus = ApplicationStatus(applicationStatus.String)
	} else {
		r.ApplicationStatus = StatusNew
	}
	r.Notified = notified != 0
	r.VerdictsJSON = scanNullString(verdictsJSON)
	return &r, nil
}
