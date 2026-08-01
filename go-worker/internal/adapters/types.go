package adapters

import "context"

type AdapterType string

const (
	Amazon     AdapterType = "amazon"
	Greenhouse AdapterType = "greenhouse"
	Lever      AdapterType = "lever"
)

// NormalizedJob is the common shape every adapter maps its source-specific
// job records into, before dedupe/classification/notification.
type NormalizedJob struct {
	ExternalID         string
	Title              string
	Location           *string
	PostedDate         *string
	QualificationsText string
	ApplyURL           string
	Raw                []byte // original source record, stored as raw_json
}

type Adapter interface {
	Type() AdapterType
	FetchJobs(ctx context.Context, config []byte, portalURL string) ([]NormalizedJob, error)
}
