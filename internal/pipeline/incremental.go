package pipeline

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"mf-analytics-service/internal/analytics"
	"mf-analytics-service/internal/db"
)

func (r *BackfillRunner) incrementalOne(ctx context.Context, st db.SyncState) error {
	// If we have never synced this scheme, fall back to a full backfill.
	if !st.LastSyncedDate.Valid {
		return r.backfillOne(ctx, st)
	}

	code64, err := strconv.ParseInt(st.SchemeCode, 10, 64)
	if err != nil {
		return r.failSyncState(ctx, st, fmt.Errorf("invalid scheme_code %q: %w", st.SchemeCode, err))
	}

	start := st.LastSyncedDate.Time.UTC().AddDate(0, 0, 1)
	end := time.Now().UTC()
	// startDate/endDate are inclusive; if start is after end, nothing to do.
	if start.After(end) {
		return db.New(r.pool).UpdateSyncStateSuccess(ctx, db.UpdateSyncStateSuccessParams{
			SchemeCode:     st.SchemeCode,
			LastSyncedDate: st.LastSyncedDate,
		})
	}

	resp, err := r.mf.GetSchemeRange(ctx, code64, start, end)
	if err != nil {
		return r.failSyncState(ctx, st, err)
	}

	q := db.New(r.pool)
	maxDate := st.LastSyncedDate.Time.UTC()

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

		if dt.After(maxDate) {
			maxDate = dt
		}
	}

	if err := analytics.ComputeAndUpsert(ctx, r.pool, st.SchemeCode); err != nil {
		return r.failSyncState(ctx, st, fmt.Errorf("compute analytics: %w", err))
	}

	return q.UpdateSyncStateSuccess(ctx, db.UpdateSyncStateSuccessParams{
		SchemeCode:     st.SchemeCode,
		LastSyncedDate: pgtype.Date{Time: maxDate, Valid: true},
	})
}

