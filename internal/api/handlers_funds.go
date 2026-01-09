package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"mf-analytics-service/internal/db"
)

func (s *Server) handleFundsList() http.HandlerFunc {
	type fund struct {
		SchemeCode string `json:"scheme_code"`
		SchemeName string `json:"scheme_name"`
		AMC        string `json:"amc"`
		Category   string `json:"category"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		category := r.URL.Query().Get("category")
		amc := r.URL.Query().Get("amc")

		q := db.New(s.pool)
		rows, err := q.ListFunds(r.Context(), db.ListFundsParams{
			Category: pgtype.Text{String: category, Valid: category != ""},
			Amc:      pgtype.Text{String: amc, Valid: amc != ""},
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		out := make([]fund, 0, len(rows))
		for _, f := range rows {
			out = append(out, fund{
				SchemeCode: f.SchemeCode,
				SchemeName: f.SchemeName,
				AMC:        f.Amc,
				Category:   f.Category,
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{"funds": out})
	}
}

func (s *Server) handleFundDetails() http.HandlerFunc {
	type resp struct {
		SchemeCode string  `json:"scheme_code"`
		SchemeName string  `json:"scheme_name"`
		AMC        string  `json:"amc"`
		Category   string  `json:"category"`
		LatestNAV  float64 `json:"latest_nav,omitempty"`
		NAVDate    string  `json:"nav_date,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "code")
		if code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing fund code"})
			return
		}

		q := db.New(s.pool)
		f, err := q.GetFund(r.Context(), code)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "fund not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		out := resp{
			SchemeCode: f.SchemeCode,
			SchemeName: f.SchemeName,
			AMC:        f.Amc,
			Category:   f.Category,
		}

		if nav, err := q.GetLatestNav(r.Context(), code); err == nil {
			out.LatestNAV = nav.NavValue.InexactFloat64()
			if nav.NavDate.Valid {
				out.NAVDate = nav.NavDate.Time.UTC().Format("2006-01-02")
			}
		}

		writeJSON(w, http.StatusOK, out)
	}
}

func parseLimit(s string, def int32) (int32, error) {
	if s == "" {
		return def, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return 0, errors.New("limit must be a positive integer")
	}
	return int32(v), nil
}
