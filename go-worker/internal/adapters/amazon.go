package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/syumai/workers/cloudflare/fetch"
)

type amazonJobRecord struct {
	ID                     string `json:"id"`
	IDIcims                string `json:"id_icims"`
	Title                  string `json:"title"`
	Location               string `json:"location"`
	NormalizedLocation     string `json:"normalized_location"`
	PostedDate             string `json:"posted_date"`
	BasicQualifications    string `json:"basic_qualifications"`
	PreferredQualifications string `json:"preferred_qualifications"`
	Description            string `json:"description"`
	JobPath                string `json:"job_path"`
}

type amazonSearchResponse struct {
	Hits int               `json:"hits"`
	Jobs []amazonJobRecord `json:"jobs"`
}

type AmazonAdapterConfig struct {
	ResultLimit int `json:"resultLimit"`
	MaxResults  int `json:"maxResults"`
}

const (
	amazonJobsOrigin       = "https://www.amazon.jobs"
	amazonDefaultPageSize  = 20
	amazonDefaultMaxResult = 200
)

type amazonAdapter struct {
	client *fetch.Client
}

func NewAmazonAdapter(client *fetch.Client) Adapter {
	return &amazonAdapter{client: client}
}

func (a *amazonAdapter) Type() AdapterType { return Amazon }

func toAmazonNormalizedJob(job amazonJobRecord) NormalizedJob {
	externalID := job.IDIcims
	if externalID == "" {
		externalID = job.ID
	}

	var qualParts []string
	for _, part := range []string{job.BasicQualifications, job.PreferredQualifications, job.Description} {
		if strings.TrimSpace(part) != "" {
			qualParts = append(qualParts, part)
		}
	}

	location := job.NormalizedLocation
	if location == "" {
		location = job.Location
	}
	var locationPtr *string
	if location != "" {
		locationPtr = &location
	}
	var postedDatePtr *string
	if job.PostedDate != "" {
		postedDatePtr = &job.PostedDate
	}

	applyURL := fmt.Sprintf("%s/en/jobs/%s", amazonJobsOrigin, externalID)
	if job.JobPath != "" {
		applyURL = amazonJobsOrigin + job.JobPath
	}

	raw, _ := json.Marshal(job)

	return NormalizedJob{
		ExternalID:         externalID,
		Title:              job.Title,
		Location:           locationPtr,
		PostedDate:         postedDatePtr,
		QualificationsText: strings.Join(qualParts, "\n\n"),
		ApplyURL:           applyURL,
		Raw:                raw,
	}
}

func (a *amazonAdapter) FetchJobs(ctx context.Context, configJSON []byte, portalURL string) ([]NormalizedJob, error) {
	var cfg AmazonAdapterConfig
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return nil, fmt.Errorf("invalid amazon adapter config: %w", err)
		}
	}
	pageSize := cfg.ResultLimit
	if pageSize <= 0 {
		pageSize = amazonDefaultPageSize
	}
	maxResults := cfg.MaxResults
	if maxResults <= 0 {
		maxResults = amazonDefaultMaxResult
	}

	base, err := url.Parse(portalURL)
	if err != nil {
		return nil, fmt.Errorf("invalid amazon portal url: %w", err)
	}

	var results []NormalizedJob
	offset := 0

	for len(results) < maxResults {
		q := base.Query()
		q.Set("result_limit", strconv.Itoa(pageSize))
		q.Set("offset", strconv.Itoa(offset))
		reqURL := *base
		reqURL.RawQuery = q.Encode()

		req, err := fetch.NewRequest(ctx, http.MethodGet, reqURL.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")

		resp, err := a.client.Do(req, nil)
		if err != nil {
			return nil, fmt.Errorf("amazon jobs search request failed: %w", err)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return nil, fmt.Errorf("amazon jobs search failed: %s", resp.Status)
		}

		var data amazonSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode amazon jobs response: %w", err)
		}
		resp.Body.Close()

		for _, job := range data.Jobs {
			results = append(results, toAmazonNormalizedJob(job))
		}

		if len(data.Jobs) < pageSize || offset+pageSize >= data.Hits {
			break
		}
		offset += pageSize
	}

	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results, nil
}
