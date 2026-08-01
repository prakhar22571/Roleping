package db

import "roleping-worker/internal/adapters"

type User struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
	// SessionEpoch is bumped on password change to invalidate old cookies.
	SessionEpoch int64 `json:"-"`
	// Credentials are never serialized to JSON.
	PasswordHash       string `json:"-"`
	PasswordSalt       string `json:"-"`
	PasswordIterations int    `json:"-"`
}

// HasPassword reports whether the user can log in. Users created by the
// pipeline or by an owner before a password is assigned cannot.
func (u User) HasPassword() bool {
	return u.PasswordHash != "" && u.PasswordSalt != ""
}

type Company struct {
	ID            int64                `json:"id"`
	Name          string               `json:"name"`
	PortalURL     string               `json:"portal_url"`
	AdapterType   adapters.AdapterType `json:"adapter_type"`
	AdapterConfig *string              `json:"adapter_config"`
	IsActive      bool                 `json:"is_active"`
	CreatedAt     string               `json:"created_at"`
	UpdatedAt     string               `json:"updated_at"`
}

type Job struct {
	ID                 int64                `json:"id"`
	ExternalID         string               `json:"external_id"`
	Source             adapters.AdapterType `json:"source"`
	CompanyID          int64                `json:"company_id"`
	Title              string               `json:"title"`
	Location           *string              `json:"location"`
	PostedDate         *string              `json:"posted_date"`
	QualificationsText *string              `json:"qualifications_text"`
	ApplyURL           string               `json:"apply_url"`
	RawJSON            *string              `json:"raw_json"`
	FirstSeenAt        string               `json:"first_seen_at"`
}

type LlmVerdict struct {
	ID           int64    `json:"id"`
	JobID        int64    `json:"job_id"`
	ModelID      string   `json:"model_id"`
	Match        bool     `json:"match"`
	Confidence   *float64 `json:"confidence"`
	Reasoning    string   `json:"reasoning"`
	RawResponse  *string  `json:"raw_response"`
	ClassifiedAt string   `json:"classified_at"`
}

type EmailStatus string

const (
	EmailSent   EmailStatus = "sent"
	EmailFailed EmailStatus = "failed"
	// EmailNone marks a dashboard-only notification: the subscriber is not
	// the owner, so no email was attempted.
	EmailNone EmailStatus = "none"
)

type Notification struct {
	ID          int64       `json:"id"`
	JobID       int64       `json:"job_id"`
	UserID      int64       `json:"user_id"`
	SentAt      string      `json:"sent_at"`
	EmailStatus EmailStatus `json:"email_status"`
	ResendID    *string     `json:"resend_id"`
	ErrorDetail *string     `json:"error_detail"`
}

// NotificationRow is a notification joined with its job and company for
// list views.
type NotificationRow struct {
	Notification
	JobTitle    string `json:"job_title"`
	CompanyName string `json:"company_name"`
	ApplyURL    string `json:"apply_url"`
}

// CompanyWithSubscription is a company plus whether a given user is
// subscribed to it.
type CompanyWithSubscription struct {
	Company
	Subscribed bool `json:"subscribed"`
}

type ApplicationStatus string

const (
	StatusNew          ApplicationStatus = "New"
	StatusApplied      ApplicationStatus = "Applied"
	StatusInterviewing ApplicationStatus = "Interviewing"
	StatusRejected     ApplicationStatus = "Rejected"
	StatusOffer        ApplicationStatus = "Offer"
)

var ValidApplicationStatuses = []ApplicationStatus{
	StatusNew, StatusApplied, StatusInterviewing, StatusRejected, StatusOffer,
}

func IsValidApplicationStatus(s string) bool {
	for _, valid := range ValidApplicationStatuses {
		if string(valid) == s {
			return true
		}
	}
	return false
}

var ValidAdapterTypes = []adapters.AdapterType{
	adapters.Amazon, adapters.Greenhouse, adapters.Lever,
}

func IsValidAdapterType(s string) bool {
	for _, valid := range ValidAdapterTypes {
		if string(valid) == s {
			return true
		}
	}
	return false
}
