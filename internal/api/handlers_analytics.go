package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"mf-analytics-service/internal/db"
)

func (s *Server) handleFundAnalytics() http.HandlerFunc {
	type resp struct {
		FundCode string `json:"fund_code"`
		FundName string `json:"fund_name"`
		Category string `json:"category"`
		AMC      string `json:"amc"`
		Window   string `json:"window"`

		DataAvailability struct {
			StartDate     string `json:"start_date,omitempty"`
			EndDate       string `json:"end_date,omitempty"`
			TotalDays     int    `json:"total_days,omitempty"`
			NavDataPoints int    `json:"nav_data_points,omitempty"`
		} `json:"data_availability"`

		RollingPeriodsAnalyzed int `json:"rolling_periods_analyzed"`

		RollingReturns struct {
			Min    *float64 `json:"min,omitempty"`
			Max    *float64 `json:"max,omitempty"`
			Median *float64 `json:"median,omitempty"`
			P25    *float64 `json:"p25,omitempty"`
			P75    *float64 `json:"p75,omitempty"`
		} `json:"rolling_returns"`

		MaxDrawdown *float64 `json:"max_drawdown,omitempty"`

		CAGR struct {
			Min    *float64 `json:"min,omitempty"`
			Max    *float64 `json:"max,omitempty"`
			Median *float64 `json:"median,omitempty"`
		} `json:"cagr"`

		ComputedAt string `json:"computed_at,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "code")
		window := strings.TrimSpace(r.URL.Query().Get("window"))
		if code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing fund code"})
			return
		}
		if !isValidWindow(window) {
			writeJSON(
				w,
				http.StatusBadRequest,
				map[string]any{"error": "window must be one of 1Y|3Y|5Y|10Y"},
			)
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

		a, err := q.GetFundAnalytics(
			r.Context(),
			db.GetFundAnalyticsParams{SchemeCode: code, Window: window},
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(
					w,
					http.StatusNotFound,
					map[string]any{"error": "analytics not computed yet"},
				)
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		out := resp{
			FundCode: code,
			FundName: f.SchemeName,
			Category: f.Category,
			AMC:      f.Amc,
			Window:   window,
		}

		if a.DataStartDate.Valid {
			out.DataAvailability.StartDate = a.DataStartDate.Time.UTC().Format("2006-01-02")
		}
		if a.DataEndDate.Valid {
			out.DataAvailability.EndDate = a.DataEndDate.Time.UTC().Format("2006-01-02")
		}
		if a.DataStartDate.Valid && a.DataEndDate.Valid {
			out.DataAvailability.TotalDays = int(
				a.DataEndDate.Time.Sub(a.DataStartDate.Time).Hours()/24,
			) + 1
		}
		if a.NavPoints.Valid {
			out.DataAvailability.NavDataPoints = int(a.NavPoints.Int32)
		}
		if a.RollingPeriods.Valid {
			out.RollingPeriodsAnalyzed = int(a.RollingPeriods.Int32)
		}

		out.RollingReturns.Min = numericPtr(a.RollingMin)
		out.RollingReturns.Max = numericPtr(a.RollingMax)
		out.RollingReturns.Median = numericPtr(a.RollingMedian)
		out.RollingReturns.P25 = numericPtr(a.RollingP25)
		out.RollingReturns.P75 = numericPtr(a.RollingP75)

		out.MaxDrawdown = numericPtr(a.MaxDrawdown)

		out.CAGR.Min = numericPtr(a.CagrMin)
		out.CAGR.Max = numericPtr(a.CagrMax)
		out.CAGR.Median = numericPtr(a.CagrMedian)

		if a.ComputedAt.Valid {
			out.ComputedAt = a.ComputedAt.Time.UTC().Format(timeRFC3339)
		}

		writeJSON(w, http.StatusOK, out)
	}
}

func isValidWindow(w string) bool {
	switch w {
	case "1Y", "3Y", "5Y", "10Y":
		return true
	default:
		return false
	}
}
