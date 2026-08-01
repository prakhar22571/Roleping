package main

import (
	"context"
	"fmt"

	"github.com/syumai/workers"
	_ "github.com/syumai/workers/cloudflare/d1"
	"github.com/syumai/workers/cloudflare/cron"

	"roleping-worker/internal/adapters"
	"roleping-worker/internal/config"
	"roleping-worker/internal/jobs"
	"roleping-worker/internal/router"
)

func runScheduledPipeline(ctx context.Context) error {
	env, err := config.Load()
	if err != nil {
		return err
	}

	client := config.NewFetchClient()
	registry := adapters.NewRegistry(client)

	summary, err := jobs.RunPipeline(ctx, env, registry, client)
	if err != nil {
		return err
	}

	fmt.Printf("Pipeline run summary: processed=%d failed=%d newJobs=%d notificationsSent=%d\n",
		summary.CompaniesProcessed, summary.CompaniesFailed, summary.NewJobs, summary.NotificationsSent)
	return nil
}

func main() {
	handler := router.New()

	workers.ServeNonBlock(handler)
	cron.ScheduleTaskNonBlock(runScheduledPipeline)

	workers.Ready()

	select {
	case <-workers.Done():
	case <-cron.Done():
	}
}
