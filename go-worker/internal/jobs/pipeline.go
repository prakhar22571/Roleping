package jobs

import (
	"context"
	"fmt"
	"strings"

	"github.com/syumai/workers/cloudflare/fetch"

	"roleping-worker/internal/adapters"
	"roleping-worker/internal/classification"
	"roleping-worker/internal/config"
	"roleping-worker/internal/db"
	"roleping-worker/internal/notify"
)

type PipelineSummary struct {
	CompaniesProcessed int `json:"companiesProcessed"`
	CompaniesFailed    int `json:"companiesFailed"`
	NewJobs            int `json:"newJobs"`
	NotificationsSent  int `json:"notificationsSent"`
}

func RunPipeline(ctx context.Context, env *config.Env, registry *adapters.Registry, client *fetch.Client) (PipelineSummary, error) {
	var summary PipelineSummary

	modelA, modelB := env.ModelIDs()

	// The cron may fire before the owner has ever logged in; make sure the
	// owner's user row exists so their subscriptions can be recorded.
	if env.OwnerEmail != "" {
		if _, err := db.GetOrCreateUser(ctx, env.DB, env.OwnerEmail); err != nil {
			return summary, err
		}
	}

	companies, err := db.ListActiveCompanies(ctx, env.DB)
	if err != nil {
		return summary, err
	}

	for _, company := range companies {
		if err := processCompany(ctx, env, registry, client, company, modelA, modelB, &summary); err != nil {
			summary.CompaniesFailed++
			fmt.Printf("Pipeline failed for company %d (%s): %v\n", company.ID, company.Name, err)
			continue
		}
		summary.CompaniesProcessed++
	}

	return summary, nil
}

func processCompany(
	ctx context.Context,
	env *config.Env,
	registry *adapters.Registry,
	client *fetch.Client,
	company db.Company,
	modelA, modelB string,
	summary *PipelineSummary,
) error {
	adapter, err := registry.Get(company.AdapterType)
	if err != nil {
		return err
	}

	var configJSON []byte
	if company.AdapterConfig != nil {
		configJSON = []byte(*company.AdapterConfig)
	}

	normalizedJobs, err := adapter.FetchJobs(ctx, configJSON, company.PortalURL)
	if err != nil {
		return err
	}

	subscribers, err := db.ListSubscribersForCompany(ctx, env.DB, company.ID)
	if err != nil {
		return err
	}

	for _, normalizedJob := range normalizedJobs {
		jobRow, err := InsertIfNew(ctx, env.DB, company.ID, company.AdapterType, normalizedJob)
		if err != nil {
			return err
		}
		if jobRow == nil {
			continue
		}
		summary.NewJobs++

		verdictA, verdictB := classification.RunClassification(ctx, client, env.OpenRouterAPIKey, modelA, modelB, normalizedJob)

		if err := db.UpsertVerdict(ctx, env.DB, toVerdictInput(jobRow.ID, verdictA)); err != nil {
			return err
		}
		if err := db.UpsertVerdict(ctx, env.DB, toVerdictInput(jobRow.ID, verdictB)); err != nil {
			return err
		}

		if verdictA.Match || verdictB.Match {
			// Every subscriber gets a dashboard notification; only the
			// owner gets an email (Resend's free sender can only deliver
			// to the account owner's address).
			for _, sub := range subscribers {
				input := db.NotificationInput{
					JobID:       jobRow.ID,
					UserID:      sub.ID,
					EmailStatus: db.EmailNone,
				}
				if strings.EqualFold(sub.Email, env.OwnerEmail) {
					result := notify.SendJobAlertEmail(ctx, client, env.ResendAPIKey, env.AlertFromEmail, sub.Email, company, normalizedJob, []classification.Verdict{verdictA, verdictB})
					input.EmailStatus = db.EmailFailed
					if result.OK {
						input.EmailStatus = db.EmailSent
						summary.NotificationsSent++
					}
					input.ResendID = result.ResendID
					input.ErrorDetail = result.ErrorDetail
				}
				if err := db.RecordNotification(ctx, env.DB, input); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func toVerdictInput(jobID int64, v classification.Verdict) db.VerdictInput {
	return db.VerdictInput{
		JobID:       jobID,
		ModelID:     v.ModelID,
		Match:       v.Match,
		Confidence:  v.Confidence,
		Reasoning:   v.Reasoning,
		RawResponse: v.RawResponse,
	}
}
