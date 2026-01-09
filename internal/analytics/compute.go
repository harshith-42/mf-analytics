package analytics

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"mf-analytics-service/internal/db"
)

type WindowSpec struct {
	Label string
	Years int
}

var DefaultWindows = []WindowSpec{
	{Label: "1Y", Years: 1},
	{Label: "3Y", Years: 3},
	{Label: "5Y", Years: 5},
	{Label: "10Y", Years: 10},
}

type point struct {
	date time.Time
	nav  float64
}

// ComputeAndUpsert computes analytics for all windows for a scheme and upserts `fund_analytics`.
// If there isn't enough history for a window, it still upserts a row with availability fields and NULL metrics.
func ComputeAndUpsert(ctx context.Context, pool *pgxpool.Pool, schemeCode string) error {
	q := db.New(pool)
	rows, err := q.ListNavHistoryForScheme(ctx, schemeCode)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("no nav history for scheme_code=%s", schemeCode)
	}

	pts := make([]point, 0, len(rows))
	for _, r := range rows {
		if !r.NavDate.Valid {
			continue
		}
		nav := r.NavValue
		f, ok := decimalToFloat(nav)
		if !ok || f <= 0 {
			continue
		}
		pts = append(pts, point{date: r.NavDate.Time.UTC(), nav: f})
	}
	if len(pts) < 2 {
		return fmt.Errorf("insufficient usable nav points for scheme_code=%s", schemeCode)
	}

	// Ensure sorted (db query should already do it).
	sort.Slice(pts, func(i, j int) bool { return pts[i].date.Before(pts[j].date) })

	startDate := pts[0].date
	endDate := pts[len(pts)-1].date

	for _, w := range DefaultWindows {
		res := computeWindow(pts, w.Years)

		params := db.UpsertFundAnalyticsParams{
			SchemeCode: schemeCode,
			Window:     w.Label,

			RollingMin:    res.rollingMin,
			RollingMax:    res.rollingMax,
			RollingMedian: res.rollingMedian,
			RollingP25:    res.rollingP25,
			RollingP75:    res.rollingP75,

			MaxDrawdown: res.maxDrawdown,

			CagrMin:    res.cagrMin,
			CagrMax:    res.cagrMax,
			CagrMedian: res.cagrMedian,

			DataStartDate:  pgtype.Date{Time: startDate, Valid: true},
			DataEndDate:    pgtype.Date{Time: endDate, Valid: true},
			NavPoints:      pgtype.Int4{Int32: int32(len(pts)), Valid: true},
			RollingPeriods: pgtype.Int4{Int32: int32(res.rollingPeriods), Valid: true},
		}

		if err := q.UpsertFundAnalytics(ctx, params); err != nil {
			return err
		}
	}

	return nil
}

type windowResult struct {
	rollingPeriods int

	rollingMin    pgtype.Numeric
	rollingMax    pgtype.Numeric
	rollingMedian pgtype.Numeric
	rollingP25    pgtype.Numeric
	rollingP75    pgtype.Numeric

	maxDrawdown pgtype.Numeric

	cagrMin    pgtype.Numeric
	cagrMax    pgtype.Numeric
	cagrMedian pgtype.Numeric
}

func computeWindow(pts []point, years int) windowResult {
	// Collect per-period rolling returns and CAGRs.
	returns := make([]float64, 0, len(pts))
	cagrs := make([]float64, 0, len(pts))

	// Worst drawdown across all rolling windows of this length.
	worstDrawdown := math.Inf(1) // we'll take min (more negative)

	// i is the index of the NAV at or before (endDate - years).
	i := 0
	for j := 0; j < len(pts); j++ {
		startNeed := pts[j].date.AddDate(-years, 0, 0)
		for i+1 < j && !pts[i+1].date.After(startNeed) {
			i++
		}

		// If we don't have a point at/before startNeed, skip.
		if pts[0].date.After(startNeed) {
			continue
		}
		if i >= j {
			continue
		}

		startNav := pts[i].nav
		endNav := pts[j].nav
		if startNav <= 0 || endNav <= 0 {
			continue
		}

		r := (endNav/startNav - 1.0) * 100.0
		returns = append(returns, r)

		c := (math.Pow(endNav/startNav, 1.0/float64(years)) - 1.0) * 100.0
		if !math.IsNaN(c) && !math.IsInf(c, 0) {
			cagrs = append(cagrs, c)
		}

		dd := maxDrawdownPct(pts[i : j+1])
		if dd < worstDrawdown {
			worstDrawdown = dd
		}
	}

	res := windowResult{rollingPeriods: len(returns)}

	if len(returns) == 0 {
		// Not enough history for this window. Leave metrics NULL but keep rollingPeriods=0.
		res.rollingMin = pgtype.Numeric{Valid: false}
		res.rollingMax = pgtype.Numeric{Valid: false}
		res.rollingMedian = pgtype.Numeric{Valid: false}
		res.rollingP25 = pgtype.Numeric{Valid: false}
		res.rollingP75 = pgtype.Numeric{Valid: false}
		res.maxDrawdown = pgtype.Numeric{Valid: false}
		res.cagrMin = pgtype.Numeric{Valid: false}
		res.cagrMax = pgtype.Numeric{Valid: false}
		res.cagrMedian = pgtype.Numeric{Valid: false}
		return res
	}

	sort.Float64s(returns)
	res.rollingMin = mustNumeric(returns[0])
	res.rollingMax = mustNumeric(returns[len(returns)-1])
	res.rollingP25 = mustNumeric(percentileSorted(returns, 0.25))
	res.rollingMedian = mustNumeric(percentileSorted(returns, 0.50))
	res.rollingP75 = mustNumeric(percentileSorted(returns, 0.75))

	if math.IsInf(worstDrawdown, 1) {
		res.maxDrawdown = pgtype.Numeric{Valid: false}
	} else {
		res.maxDrawdown = mustNumeric(worstDrawdown)
	}

	if len(cagrs) == 0 {
		res.cagrMin = pgtype.Numeric{Valid: false}
		res.cagrMax = pgtype.Numeric{Valid: false}
		res.cagrMedian = pgtype.Numeric{Valid: false}
	} else {
		sort.Float64s(cagrs)
		res.cagrMin = mustNumeric(cagrs[0])
		res.cagrMax = mustNumeric(cagrs[len(cagrs)-1])
		res.cagrMedian = mustNumeric(percentileSorted(cagrs, 0.50))
	}

	return res
}

func maxDrawdownPct(window []point) float64 {
	peak := window[0].nav
	worst := 0.0
	for _, p := range window {
		if p.nav > peak {
			peak = p.nav
		}
		if peak <= 0 {
			continue
		}
		dd := (p.nav/peak - 1.0) * 100.0
		if dd < worst {
			worst = dd
		}
	}
	return worst
}

func percentileSorted(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}

	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func mustNumeric(v float64) pgtype.Numeric {
	// Keep consistent with schema precision NUMERIC(6,2): round to 2 decimals.
	s := fmt.Sprintf("%.2f", v)
	var n pgtype.Numeric
	_ = n.Scan(s)
	return n
}

func decimalToFloat(d decimal.Decimal) (float64, bool) {
	f, exact := d.Float64()
	// float64 conversion can be inexact; that's fine for percent metrics.
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, exact
}
