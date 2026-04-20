package datadog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ErrAuthFailure is returned when the Datadog API responds with 403.
var ErrAuthFailure = errors.New("datadog: authentication failed (403)")

// Series holds the last observed value for a single metric series.
type Series struct {
	Metric    string
	Scope     string
	LastValue float64
}

// Client calls the Datadog Metrics Query API.
type Client struct {
	apiKey     string
	appKey     string
	baseURL    string
	httpClient *http.Client
}

// NewClient returns a Client authenticating with apiKey and appKey.
// baseURL defaults to https://api.datadoghq.com.
func NewClient(apiKey, appKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		appKey:     appKey,
		baseURL:    "https://api.datadoghq.com",
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// withBaseURL overrides the base URL (used in tests).
func (c *Client) withBaseURL(u string) *Client {
	c.baseURL = u
	return c
}

// ddQueryResponse is a minimal representation of the Datadog query response.
type ddQueryResponse struct {
	Series []ddSeries `json:"series"`
}

type ddSeries struct {
	Metric    string      `json:"metric"`
	Scope     string      `json:"scope"`
	Pointlist [][2]float64 `json:"pointlist"`
}

// QueryLastValues queries metric data over the last window duration and returns
// the last data point for each series. Returns ErrAuthFailure on 403.
// Retries on 429 with exponential backoff (max 5 attempts, cap 60s).
func (c *Client) QueryLastValues(ctx context.Context, query string, window time.Duration) ([]Series, error) {
	now := time.Now().UTC()
	from := now.Add(-window)

	params := url.Values{
		"query": {query},
		"from":  {strconv.FormatInt(from.Unix(), 10)},
		"to":    {strconv.FormatInt(now.Unix(), 10)},
	}
	endpoint := c.baseURL + "/api/v1/query?" + params.Encode()

	backoff := time.Second
	const maxRetries = 5

	for attempt := 0; attempt < maxRetries; attempt++ {
		series, status, err := c.doQuery(ctx, endpoint)
		if err != nil {
			return nil, err
		}
		switch {
		case status == http.StatusForbidden:
			return nil, ErrAuthFailure
		case status == http.StatusTooManyRequests:
			t := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				t.Stop()
				return nil, ctx.Err()
			case <-t.C:
			}
			backoff *= 2
			if backoff > 60*time.Second {
				backoff = 60 * time.Second
			}
			continue
		case status < 200 || status >= 300:
			return nil, fmt.Errorf("datadog: query returned status %d", status)
		}
		return series, nil
	}
	return nil, fmt.Errorf("datadog: query rate-limited after %d attempts", maxRetries)
}

func (c *Client) doQuery(ctx context.Context, endpoint string) ([]Series, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("datadog: build request: %w", err)
	}
	req.Header.Set("DD-API-KEY", c.apiKey)
	req.Header.Set("DD-APPLICATION-KEY", c.appKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("datadog: GET %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, resp.StatusCode, nil
	}

	var qr ddQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("datadog: decode response: %w", err)
	}

	var out []Series
	for _, s := range qr.Series {
		last := 0.0
		if len(s.Pointlist) > 0 {
			last = s.Pointlist[len(s.Pointlist)-1][1]
		}
		out = append(out, Series{
			Metric:    s.Metric,
			Scope:     s.Scope,
			LastValue: last,
		})
	}
	return out, http.StatusOK, nil
}
