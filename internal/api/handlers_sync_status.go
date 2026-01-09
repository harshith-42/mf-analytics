package api

import (
	"net/http"

	"mf-analytics-service/internal/db"
)

func (s *Server) handleSyncStatus() http.HandlerFunc {
	type run struct {
		RunID      string `json:"run_id,omitempty"`
		RunType    string `json:"run_type,omitempty"`
		Status     string `json:"status,omitempty"`
		StartedAt  string `json:"started_at,omitempty"`
		FinishedAt string `json:"finished_at,omitempty"`
		Error      string `json:"error_summary,omitempty"`
	}

	type scheme struct {
		SchemeCode     string `json:"scheme_code"`
		Status         string `json:"status"`
		LastSyncedDate string `json:"last_synced_date,omitempty"`
		RetryCount     int32  `json:"retry_count"`
		LastError      string `json:"last_error,omitempty"`
		LastAttemptAt  string `json:"last_attempt_at,omitempty"`
	}

	type resp struct {
		LatestRun run              `json:"latest_run"`
		Counts    map[string]int64 `json:"counts"`
		Schemes   []scheme         `json:"schemes"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		q := db.New(s.pool)

		out := resp{
			Counts:  map[string]int64{},
			Schemes: []scheme{},
		}

		if latest, err := q.GetLatestSyncRun(r.Context()); err == nil {
			if latest.RunID.Valid {
				out.LatestRun.RunID = uuidFromPg(latest.RunID).String()
			}
			out.LatestRun.RunType = latest.RunType
			out.LatestRun.Status = latest.Status
			if latest.StartedAt.Valid {
				out.LatestRun.StartedAt = latest.StartedAt.Time.UTC().Format(timeRFC3339)
			}
			if latest.FinishedAt.Valid {
				out.LatestRun.FinishedAt = latest.FinishedAt.Time.UTC().Format(timeRFC3339)
			}
			if latest.ErrorSummary.Valid {
				out.LatestRun.Error = latest.ErrorSummary.String
			}
		}

		if counts, err := q.CountSyncStateByStatus(r.Context()); err == nil {
			for _, c := range counts {
				out.Counts[c.Status] = c.Count
			}
		}

		if states, err := q.ListSyncState(r.Context()); err == nil {
			for _, st := range states {
				si := scheme{
					SchemeCode: st.SchemeCode,
					Status:     st.Status,
					RetryCount: st.RetryCount,
				}
				if st.LastSyncedDate.Valid {
					si.LastSyncedDate = st.LastSyncedDate.Time.UTC().Format("2006-01-02")
				}
				if st.LastError.Valid {
					si.LastError = st.LastError.String
				}
				if st.LastAttemptAt.Valid {
					si.LastAttemptAt = st.LastAttemptAt.Time.UTC().Format(timeRFC3339)
				}
				out.Schemes = append(out.Schemes, si)
			}
		}

		writeJSON(w, http.StatusOK, out)
	}
}
