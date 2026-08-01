package jobs

import (
	"context"
	"database/sql"

	"roleping-worker/internal/adapters"
	"roleping-worker/internal/db"
)

// InsertIfNew inserts the normalized job if (source, external_id) hasn't been
// seen before, returning nil (no error) when it was already present.
func InsertIfNew(ctx context.Context, conn *sql.DB, companyID int64, source adapters.AdapterType, job adapters.NormalizedJob) (*db.Job, error) {
	existing, err := db.FindJobBySourceExternalID(ctx, conn, source, job.ExternalID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, nil
	}
	return db.InsertJob(ctx, conn, companyID, source, job)
}
