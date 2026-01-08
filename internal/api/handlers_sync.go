package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"mf-analytics-service/internal/db"
)

func (s *Server) handleSyncTrigger() http.HandlerFunc {
	type resp struct {
		RunID string `json:"run_id"`
	}
	type errResp struct {
		Error string `json:"error"`
		RunID string `json:"run_id,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		runID, status, err := s.enqueueManualRun(ctx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errResp{Error: err.Error()})
			return
		}

		if status == http.StatusConflict {
			writeJSON(w, http.StatusConflict, errResp{
				Error: "a sync run is already running",
				RunID: runID,
			})
			return
		}

		writeJSON(w, http.StatusAccepted, resp{RunID: runID})
	}
}

func (s *Server) enqueueManualRun(ctx context.Context) (runID string, status int, err error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)

	if existing, err := q.GetLatestRunningSyncRun(ctx); err == nil {
		if existing.RunID.Valid {
			return uuidFromPg(existing.RunID).String(), http.StatusConflict, nil
		}
		return "", http.StatusConflict, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", 0, err
	}

	u := uuid.New()
	pgID := pgtype.UUID{Bytes: uuidToBytes16(u), Valid: true}

	if err := q.CreateSyncRun(ctx, db.CreateSyncRunParams{
		RunID:   pgID,
		RunType: "MANUAL",
	}); err != nil {
		return "", 0, err
	}

	// Enqueue scheme work by marking non-in-progress schemes as PENDING.
	if err := q.ResetAllSyncStateToPending(ctx); err != nil {
		return "", 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", 0, err
	}
	return u.String(), http.StatusAccepted, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func uuidToBytes16(u uuid.UUID) [16]byte {
	var b [16]byte
	copy(b[:], u[:])
	return b
}

func uuidFromPg(id pgtype.UUID) uuid.UUID {
	return uuid.UUID(id.Bytes)
}
