package analytics

import (
	"math"
	"testing"
	"time"
)

func TestMaxDrawdownPct(t *testing.T) {
	pts := []point{
		{date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), nav: 100},
		{date: time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC), nav: 120},
		{date: time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC), nav: 90},
		{date: time.Date(2020, 1, 4, 0, 0, 0, 0, time.UTC), nav: 110},
	}
	dd := maxDrawdownPct(pts)
	// peak 120 -> trough 90 => -25%
	if math.Abs(dd-(-25.0)) > 0.0001 {
		t.Fatalf("expected -25.0, got %f", dd)
	}
}

func TestPercentileSorted(t *testing.T) {
	x := []float64{1, 2, 3, 4}
	if got := percentileSorted(x, 0.50); got != 2.5 {
		t.Fatalf("p50 expected 2.5, got %f", got)
	}
	if got := percentileSorted(x, 0.25); got != 1.75 {
		t.Fatalf("p25 expected 1.75, got %f", got)
	}
	if got := percentileSorted(x, 0.75); got != 3.25 {
		t.Fatalf("p75 expected 3.25, got %f", got)
	}
}

