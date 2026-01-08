package mfapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mf-analytics-service/internal/ratelimiter"
)

type Client struct {
	baseURL string
	http    *http.Client
	rl      *ratelimiter.Limiter
}

type Option func(*Client)

func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

func WithRateLimiter(rl *ratelimiter.Limiter) Option {
	return func(c *Client) { c.rl = rl }
}

func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type SchemeListItem struct {
	SchemeCode int64  `json:"schemeCode"`
	SchemeName string `json:"schemeName"`
}

type SchemeMeta struct {
	FundHouse      string `json:"fund_house"`
	SchemeType     string `json:"scheme_type"`
	SchemeCategory string `json:"scheme_category"`
	SchemeCode     int64  `json:"scheme_code"`
	SchemeName     string `json:"scheme_name"`
}

type SchemeNavRow struct {
	Date string `json:"date"` // dd-mm-yyyy
	Nav  string `json:"nav"`  // decimal in string
}

type SchemeResponse struct {
	Meta SchemeMeta     `json:"meta"`
	Data []SchemeNavRow `json:"data"`
}

func (c *Client) Search(ctx context.Context, q string) ([]SchemeListItem, error) {
	u, err := url.Parse(c.baseURL + "/mf/search")
	if err != nil {
		return nil, err
	}
	qs := u.Query()
	qs.Set("q", q)
	u.RawQuery = qs.Encode()

	var out []SchemeListItem
	if err := c.getJSON(ctx, u.String(), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetScheme(ctx context.Context, schemeCode int64) (SchemeResponse, error) {
	u := fmt.Sprintf("%s/mf/%d", c.baseURL, schemeCode)
	var out SchemeResponse
	if err := c.getJSON(ctx, u, &out); err != nil {
		return SchemeResponse{}, err
	}
	return out, nil
}

// GetSchemeRange fetches NAV data for a scheme bounded by startDate/endDate (YYYY-MM-DD),
func (c *Client) GetSchemeRange(
	ctx context.Context,
	schemeCode int64,
	startDate, endDate time.Time,
) (SchemeResponse, error) {
	u, err := url.Parse(fmt.Sprintf("%s/mf/%d", c.baseURL, schemeCode))
	if err != nil {
		return SchemeResponse{}, err
	}
	q := u.Query()
	q.Set("startDate", startDate.UTC().Format("2006-01-02"))
	q.Set("endDate", endDate.UTC().Format("2006-01-02"))
	u.RawQuery = q.Encode()

	var out SchemeResponse
	if err := c.getJSON(ctx, u.String(), &out); err != nil {
		return SchemeResponse{}, err
	}
	return out, nil
}

func (c *Client) getJSON(ctx context.Context, url string, dst any) error {
	if c.rl != nil {
		if err := c.rl.Acquire(ctx); err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mfapi %s: http %d", url, resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(dst)
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
