package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/syumai/workers/cloudflare/fetch"

	"roleping-worker/internal/adapters"
	"roleping-worker/internal/classification"
	"roleping-worker/internal/db"
)

const resendURL = "https://api.resend.com/emails"

type SendResult struct {
	OK          bool
	ResendID    *string
	ErrorDetail *string
}

type resendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

type resendEmailResponse struct {
	ID string `json:"id"`
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func buildEmailHTML(company db.Company, job adapters.NormalizedJob, verdicts []classification.Verdict) string {
	var verdictItems strings.Builder
	for _, v := range verdicts {
		matchLabel := "no match"
		if v.Match {
			matchLabel = "MATCH"
		}
		verdictItems.WriteString(fmt.Sprintf(
			"<li><strong>%s</strong>: %s — %s</li>",
			html.EscapeString(v.ModelID), matchLabel, html.EscapeString(v.Reasoning),
		))
	}

	location := "unknown"
	if job.Location != nil {
		location = *job.Location
	}

	return fmt.Sprintf(`<h2>%s at %s</h2>
<p><strong>Location:</strong> %s</p>
<p><a href="%s">Apply here</a></p>
<h3>Model verdicts</h3>
<ul>%s</ul>
<h3>Qualifications</h3>
<p>%s</p>`,
		html.EscapeString(job.Title), html.EscapeString(company.Name),
		html.EscapeString(location), job.ApplyURL, verdictItems.String(),
		html.EscapeString(truncate(job.QualificationsText, 2000)),
	)
}

func SendJobAlertEmail(ctx context.Context, client *fetch.Client, apiKey, fromEmail, toEmail string, company db.Company, job adapters.NormalizedJob, verdicts []classification.Verdict) SendResult {
	body, err := json.Marshal(resendEmailRequest{
		From:    fromEmail,
		To:      []string{toEmail},
		Subject: fmt.Sprintf("New match: %s at %s", job.Title, company.Name),
		HTML:    buildEmailHTML(company, job, verdicts),
	})
	if err != nil {
		msg := err.Error()
		return SendResult{OK: false, ErrorDetail: &msg}
	}

	req, err := fetch.NewRequest(ctx, http.MethodPost, resendURL, bytes.NewReader(body))
	if err != nil {
		msg := err.Error()
		return SendResult{OK: false, ErrorDetail: &msg}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req, nil)
	if err != nil {
		msg := err.Error()
		return SendResult{OK: false, ErrorDetail: &msg}
	}
	defer resp.Body.Close()

	respBuf := new(bytes.Buffer)
	respBuf.ReadFrom(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := fmt.Sprintf("%d: %s", resp.StatusCode, respBuf.String())
		return SendResult{OK: false, ErrorDetail: &msg}
	}

	var parsed resendEmailResponse
	if err := json.Unmarshal(respBuf.Bytes(), &parsed); err != nil {
		msg := err.Error()
		return SendResult{OK: false, ErrorDetail: &msg}
	}

	return SendResult{OK: true, ResendID: &parsed.ID}
}
