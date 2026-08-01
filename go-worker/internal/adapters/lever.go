package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/syumai/workers/cloudflare/fetch"
)

type leverCategories struct {
	Location string `json:"location"`
}

type leverList struct {
	Text    string `json:"text"`
	Content string `json:"content"`
}

type leverJobRecord struct {
	ID              string          `json:"id"`
	Text            string          `json:"text"`
	Categories      leverCategories `json:"categories"`
	CreatedAt       int64           `json:"createdAt"`
	DescriptionPlain string         `json:"descriptionPlain"`
	Lists           []leverList     `json:"lists"`
	HostedURL       string          `json:"hostedUrl"`
}

type LeverAdapterConfig struct {
	Company string `json:"company"`
}

type leverAdapter struct {
	client *fetch.Client
}

func NewLeverAdapter(client *fetch.Client) Adapter {
	return &leverAdapter{client: client}
}

func (a *leverAdapter) Type() AdapterType { return Lever }

func toLeverNormalizedJob(job leverJobRecord) NormalizedJob {
	var listParts []string
	for _, list := range job.Lists {
		if list.Content != "" {
			listParts = append(listParts, stripHTML(list.Content))
		}
	}
	var qualParts []string
	if job.DescriptionPlain != "" {
		qualParts = append(qualParts, job.DescriptionPlain)
	}
	if len(listParts) > 0 {
		qualParts = append(qualParts, strings.Join(listParts, "\n\n"))
	}

	var locationPtr *string
	if job.Categories.Location != "" {
		locationPtr = &job.Categories.Location
	}

	var postedDatePtr *string
	if job.CreatedAt > 0 {
		postedDate := time.UnixMilli(job.CreatedAt).UTC().Format(time.RFC3339)
		postedDatePtr = &postedDate
	}

	raw, _ := json.Marshal(job)

	return NormalizedJob{
		ExternalID:         job.ID,
		Title:              job.Text,
		Location:           locationPtr,
		PostedDate:         postedDatePtr,
		QualificationsText: strings.Join(qualParts, "\n\n"),
		ApplyURL:           job.HostedURL,
		Raw:                raw,
	}
}

func (a *leverAdapter) FetchJobs(ctx context.Context, configJSON []byte, _ string) ([]NormalizedJob, error) {
	var cfg LeverAdapterConfig
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return nil, fmt.Errorf("invalid lever adapter config: %w", err)
		}
	}
	if cfg.Company == "" {
		return nil, fmt.Errorf("lever adapter config missing company")
	}

	reqURL := fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", cfg.Company)
	req, err := fetch.NewRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req, nil)
	if err != nil {
		return nil, fmt.Errorf("lever postings fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("lever postings fetch failed: %s", resp.Status)
	}

	var data []leverJobRecord
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode lever response: %w", err)
	}

	jobs := make([]NormalizedJob, 0, len(data))
	for _, job := range data {
		jobs = append(jobs, toLeverNormalizedJob(job))
	}
	return jobs, nil
}
