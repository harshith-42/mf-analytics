package api

import (
	"strconv"
)

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

func strconvParseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
