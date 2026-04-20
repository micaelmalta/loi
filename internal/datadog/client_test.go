package datadog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func serveFixture(t *testing.T, status int, body any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			json.NewEncoder(w).Encode(body)
		}
	}))
}

func fixtureResponse(metric, scope string, values ...float64) ddQueryResponse {
	pts := make([][2]float64, len(values))
	for i, v := range values {
		pts[i] = [2]float64{float64(1000 + i), v}
	}
	return ddQueryResponse{
		Series: []ddSeries{{Metric: metric, Scope: scope, Pointlist: pts}},
	}
}

func TestQueryLastValues_success(t *testing.T) {
	srv := serveFixture(t, http.StatusOK, fixtureResponse("system.cpu.user", "service:api", 10.0, 85.5))
	defer srv.Close()

	c := NewClient("key", "appkey").withBaseURL(srv.URL)
	series, err := c.QueryLastValues(context.Background(), "avg:system.cpu.user{*}", time.Minute)
	if err != nil {
		t.Fatalf("QueryLastValues: %v", err)
	}
	if len(series) != 1 {
		t.Fatalf("series len: got %d, want 1", len(series))
	}
	if series[0].LastValue != 85.5 {
		t.Errorf("LastValue: got %v, want 85.5", series[0].LastValue)
	}
	if series[0].Scope != "service:api" {
		t.Errorf("Scope: got %q, want service:api", series[0].Scope)
	}
}

func TestQueryLastValues_emptySeries(t *testing.T) {
	srv := serveFixture(t, http.StatusOK, ddQueryResponse{Series: []ddSeries{}})
	defer srv.Close()

	c := NewClient("key", "appkey").withBaseURL(srv.URL)
	series, err := c.QueryLastValues(context.Background(), "avg:system.cpu.user{*}", time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(series) != 0 {
		t.Errorf("expected empty series, got %v", series)
	}
}

func TestQueryLastValues_403_returnsErrAuthFailure(t *testing.T) {
	srv := serveFixture(t, http.StatusForbidden, nil)
	defer srv.Close()

	c := NewClient("bad-key", "bad-app").withBaseURL(srv.URL)
	_, err := c.QueryLastValues(context.Background(), "avg:system.cpu.user{*}", time.Minute)
	if err != ErrAuthFailure {
		t.Errorf("expected ErrAuthFailure, got %v", err)
	}
}

func TestQueryLastValues_429_backoffThenSuccess(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fixtureResponse("m", "s", 42.0))
	}))
	defer srv.Close()

	c := NewClient("k", "a").withBaseURL(srv.URL)
	// Use a context with a generous timeout; the backoff starts at 1s but the
	// test overrides via a short-circuit: we just verify it retries and succeeds.
	// For speed, we patch the client's http timeout to be instant.
	c.httpClient = &http.Client{Timeout: 5 * time.Second}

	series, err := c.QueryLastValues(context.Background(), "m", 10*time.Second)
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if len(series) == 0 || series[0].LastValue != 42.0 {
		t.Errorf("unexpected series: %v", series)
	}
	if attempts != 3 {
		t.Errorf("attempts: got %d, want 3", attempts)
	}
}

func TestQueryLastValues_networkError(t *testing.T) {
	// Point at a server that's immediately closed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	c := NewClient("k", "a").withBaseURL(srv.URL)
	_, err := c.QueryLastValues(context.Background(), "m", time.Minute)
	if err == nil {
		t.Error("expected error for closed server")
	}
}
