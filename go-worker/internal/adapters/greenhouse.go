package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"

	"github.com/syumai/workers/cloudflare/fetch"
)

type greenhouseLocation struct {
	Name string `json:"name"`
}

type greenhouseJobRecord struct {
	ID             int64              `json:"id"`
	Title          string             `json:"title"`
	Location       greenhouseLocation `json:"location"`
	UpdatedAt      string             `json:"updated_at"`
	FirstPublished string             `json:"first_published"`
	AbsoluteURL    string             `json:"absolute_url"`
	Content        string             `json:"content"`
}

type greenhouseBoardResponse struct {
	Jobs []greenhouseJobRecord `json:"jobs"`
}

type GreenhouseAdapterConfig struct {
	BoardToken string `json:"boardToken"`
}

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)
var whitespacePattern = regexp.MustCompile(`[\s\x{00A0}]+`)

// stripHTML unescapes HTML entities before stripping tags, since some
// sources (e.g. Greenhouse) return their "content" field with tags
// entity-encoded (e.g. "&lt;div&gt;") rather than as literal "<div>" markup -
// stripping tags first would leave the escaped markup as visible text.
func stripHTML(rawContent string) string {
	unescaped := html.UnescapeString(rawContent)
	text := htmlTagPattern.ReplaceAllString(unescaped, " ")
	text = whitespacePattern.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

type greenhouseAdapter struct {
	client *fetch.Client
}

func NewGreenhouseAdapter(client *fetch.Client) Adapter {
	return &greenhouseAdapter{client: client}
}

func (a *greenhouseAdapter) Type() AdapterType { return Greenhouse }

func toGreenhouseNormalizedJob(job greenhouseJobRecord) NormalizedJob {
	var locationPtr *string
	if job.Location.Name != "" {
		locationPtr = &job.Location.Name
	}

	postedDate := job.FirstPublished
	if postedDate == "" {
		postedDate = job.UpdatedAt
	}
	var postedDatePtr *string
	if postedDate != "" {
		postedDatePtr = &postedDate
	}

	raw, _ := json.Marshal(job)

	return NormalizedJob{
		ExternalID:         fmt.Sprintf("%d", job.ID),
		Title:              job.Title,
		Location:           locationPtr,
		PostedDate:         postedDatePtr,
		QualificationsText: stripHTML(job.Content),
		ApplyURL:           job.AbsoluteURL,
		Raw:                raw,
	}
}

func (a *greenhouseAdapter) FetchJobs(ctx context.Context, configJSON []byte, _ string) ([]NormalizedJob, error) {
	var cfg GreenhouseAdapterConfig
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return nil, fmt.Errorf("invalid greenhouse adapter config: %w", err)
		}
	}
	if cfg.BoardToken == "" {
		return nil, fmt.Errorf("greenhouse adapter config missing boardToken")
	}

	reqURL := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs?content=true", cfg.BoardToken)
	req, err := fetch.NewRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req, nil)
	if err != nil {
		return nil, fmt.Errorf("greenhouse board fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("greenhouse board fetch failed: %s", resp.Status)
	}

	var data greenhouseBoardResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode greenhouse response: %w", err)
	}

	jobs := make([]NormalizedJob, 0, len(data.Jobs))
	for _, job := range data.Jobs {
		jobs = append(jobs, toGreenhouseNormalizedJob(job))
	}
	return jobs, nil
}
