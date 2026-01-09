package api

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"mf-analytics-service/internal/db"
)

func (s *Server) handleFundsRank() http.HandlerFunc {
	type fund struct {
		Rank         int      `json:"rank"`
		FundCode     string   `json:"fund_code"`
		FundName     string   `json:"fund_name"`
		AMC          string   `json:"amc"`
		MedianReturn *float64 `json:"median_return,omitempty"`
		MaxDrawdown  *float64 `json:"max_drawdown,omitempty"`
		CurrentNAV   *float64 `json:"current_nav,omitempty"`
		LastUpdated  string   `json:"last_updated,omitempty"`
	}

	type resp struct {
		Category   string `json:"category"`
		Window     string `json:"window"`
		SortedBy   string `json:"sorted_by"`
		TotalFunds int64  `json:"total_funds"`
		Showing    int    `json:"showing"`
		Funds      []fund `json:"funds"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		category := strings.TrimSpace(r.URL.Query().Get("category"))
		window := strings.TrimSpace(r.URL.Query().Get("window"))
		sortBy := strings.TrimSpace(r.URL.Query().Get("sort_by"))
		limitStr := strings.TrimSpace(r.URL.Query().Get("limit"))

		if category == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "category is required"})
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
		if sortBy == "" {
			sortBy = "median_return"
		}
		if sortBy != "median_return" && sortBy != "max_drawdown" {
			writeJSON(
				w,
				http.StatusBadRequest,
				map[string]any{"error": "sort_by must be one of median_return|max_drawdown"},
			)
			return
		}

		limit, err := parseLimit(limitStr, 5)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}

		q := db.New(s.pool)
		total, err := q.CountFundsByCategory(r.Context(), category)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		out := resp{
			Category:   category,
			Window:     window,
			SortedBy:   sortBy,
			TotalFunds: total,
		}

		if sortBy == "max_drawdown" {
			rows, err := q.RankFundsByMaxDrawdown(r.Context(), db.RankFundsByMaxDrawdownParams{
				Category: category,
				Window:   window,
				Limit:    limit,
			})
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			out.Funds = make([]fund, 0, len(rows))
			for i, row := range rows {
				f := fund{
					Rank:         i + 1,
					FundCode:     row.SchemeCode,
					FundName:     row.SchemeName,
					AMC:          row.Amc,
					MedianReturn: numericPtr(row.RollingMedian),
					MaxDrawdown:  numericPtr(row.MaxDrawdown),
					CurrentNAV:   decimalPtr(row.CurrentNav),
				}
				if row.LastUpdated.Valid {
					f.LastUpdated = row.LastUpdated.Time.UTC().Format("2006-01-02")
				}
				out.Funds = append(out.Funds, f)
			}
		} else {
			rows, err := q.RankFundsByMedianReturn(r.Context(), db.RankFundsByMedianReturnParams{
				Category: category,
				Window:   window,
				Limit:    limit,
			})
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			out.Funds = make([]fund, 0, len(rows))
			for i, row := range rows {
				f := fund{
					Rank:         i + 1,
					FundCode:     row.SchemeCode,
					FundName:     row.SchemeName,
					AMC:          row.Amc,
					MedianReturn: numericPtr(row.RollingMedian),
					MaxDrawdown:  numericPtr(row.MaxDrawdown),
					CurrentNAV:   decimalPtr(row.CurrentNav),
				}
				if row.LastUpdated.Valid {
					f.LastUpdated = row.LastUpdated.Time.UTC().Format("2006-01-02")
				}
				out.Funds = append(out.Funds, f)
			}
		}

		out.Showing = len(out.Funds)
		writeJSON(w, http.StatusOK, out)
	}
}

func decimalPtr(d any) *float64 {
	switch v := d.(type) {
	case interface{ InexactFloat64() float64 }:
		f := v.InexactFloat64()
		return &f
	default:
		return nil
	}
}

func numericPtr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	v, err := n.Value()
	if err != nil {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return nil
	}
	f, err := strconvParseFloat(s)
	if err != nil {
		return nil
	}
	return &f
}
