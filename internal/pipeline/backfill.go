package pipeline

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"mf-analytics-service/internal/analytics"
	"mf-analytics-service/internal/db"
	"mf-analytics-service/internal/mfapi"
)

type BackfillRunner struct {
	pool       *pgxpool.Pool
	mf         *mfapi.Client
	staleAfter time.Duration
}

func NewBackfillRunner(
	pool *pgxpool.Pool,
	mf *mfapi.Client,
	staleAfter time.Duration,
) *BackfillRunner {
	if staleAfter <= 0 {
		staleAfter = 15 * time.Minute
	}
	return &BackfillRunner{pool: pool, mf: mf, staleAfter: staleAfter}
}

// RunLatest processes the latest RUNNING run until drained and marks it completed/failed.
// It returns processed=false if there is no RUNNING run.
func (r *BackfillRunner) RunLatest(ctx context.Context) (processed bool, err error) {
	q := db.New(r.pool)

	run, err := q.GetLatestRunningSyncRun(ctx)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	processed = true

	// Requeue schemes left IN_PROGRESS by previous crashed workers.
	cutoff := time.Now().Add(-r.staleAfter)
	if err := q.RequeueStaleInProgressSyncState(ctx, pgtype.Timestamp{Time: cutoff, Valid: true}); err != nil {
		return processed, err
	}

	for {
		st, err := q.ClaimNextSyncState(ctx)
		if err != nil {
			if err == pgx.ErrNoRows {
				// drained; mark overall run status based on per-scheme outcomes.
				counts, err := q.CountSyncStateByStatus(ctx)
				if err != nil {
					return processed, err
				}
				var failed int64
				for _, c := range counts {
					if c.Status == "FAILED" {
						failed = c.Count
					}
				}
				if failed > 0 {
					return processed, q.FinishSyncRunFailure(ctx, db.FinishSyncRunFailureParams{
						RunID: run.RunID,
						ErrorSummary: pgtype.Text{
							String: fmt.Sprintf("%d scheme(s) failed", failed),
							Valid:  true,
						},
					})
				}
				return processed, q.FinishSyncRunSuccess(ctx, run.RunID)
			}
			return processed, err
		}

		var perSchemeErr error
		switch run.RunType {
		case "INCREMENTAL":
			perSchemeErr = r.incrementalOne(ctx, st)
		default:
			// MANUAL and BACKFILL behave like full backfill.
			perSchemeErr = r.backfillOne(ctx, st)
		}

		if perSchemeErr != nil {
			// keep going; run can still succeed even if some schemes fail (status will show FAILED).
			// We only mark the overall run failed if DB errors prevent progress.
			continue
		}
	}
}

func (r *BackfillRunner) backfillOne(ctx context.Context, st db.SyncState) error {
	code64, err := strconv.ParseInt(st.SchemeCode, 10, 64)
	if err != nil {
		return r.failSyncState(
			ctx,
			st,
			fmt.Errorf("invalid scheme_code %q: %w", st.SchemeCode, err),
		)
	}

	resp, err := r.mf.GetScheme(ctx, code64)
	if err != nil {
		return r.failSyncState(ctx, st, err)
	}

	var maxDate time.Time
	var navPoints int

	q := db.New(r.pool)
	for _, row := range resp.Data {
		dt, err := time.Parse("02-01-2006", row.Date)
		if err != nil {
			return r.failSyncState(ctx, st, fmt.Errorf("parse date %q: %w", row.Date, err))
		}
		v, err := decimal.NewFromString(row.Nav)
		if err != nil {
			return r.failSyncState(ctx, st, fmt.Errorf("parse nav %q: %w", row.Nav, err))
		}

		if err := q.UpsertNavHistory(ctx, db.UpsertNavHistoryParams{
			SchemeCode: st.SchemeCode,
			NavDate:    pgtype.Date{Time: dt, Valid: true},
			NavValue:   v,
		}); err != nil {
			return r.failSyncState(ctx, st, err)
		}

		navPoints++
		if dt.After(maxDate) {
			maxDate = dt
		}
	}

	if navPoints == 0 {
		// Consider this a soft failure: scheme exists but no data.
		return r.failSyncState(ctx, st, fmt.Errorf("no nav data returned"))
	}

	if err := analytics.ComputeAndUpsert(ctx, r.pool, st.SchemeCode); err != nil {
		return r.failSyncState(ctx, st, fmt.Errorf("compute analytics: %w", err))
	}

	return q.UpdateSyncStateSuccess(ctx, db.UpdateSyncStateSuccessParams{
		SchemeCode:     st.SchemeCode,
		LastSyncedDate: pgtype.Date{Time: maxDate, Valid: true},
	})
}

func (r *BackfillRunner) failSyncState(ctx context.Context, st db.SyncState, cause error) error {
	q := db.New(r.pool)
	msg := cause.Error()
	_ = q.UpdateSyncStateAttempt(ctx, db.UpdateSyncStateAttemptParams{
		SchemeCode: st.SchemeCode,
		Status:     "FAILED",
		RetryCount: st.RetryCount + 1,
		LastError:  pgtype.Text{String: msg, Valid: true},
	})
	return cause
}
