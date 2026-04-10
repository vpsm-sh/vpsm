package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nathanbeddoewebdev/vpsm/internal/server/domain"

	"github.com/google/go-cmp/cmp"
)

// testMetricsResponse builds a Hetzner API metrics response with the given
// time series data. Keys follow the Hetzner API naming (e.g. "cpu",
// "disk.0.iops.read", "network.0.bandwidth.in").
func testMetricsResponse(start, end string, step float64, timeSeries map[string][][2]any) map[string]any {
	ts := make(map[string]any, len(timeSeries))
	for name, points := range timeSeries {
		values := make([]any, len(points))
		for i, pt := range points {
			values[i] = []any{pt[0], pt[1]}
		}
		ts[name] = map[string]any{
			"values": values,
		}
	}

	return map[string]any{
		"metrics": map[string]any{
			"start":       start,
			"end":         end,
			"step":        step,
			"time_series": ts,
		},
	}
}

func TestGetServerMetrics_HappyPath(t *testing.T) {
	response := testMetricsResponse(
		"2024-01-15T12:00:00Z",
		"2024-01-15T13:00:00Z",
		60,
		map[string][][2]any{
			"cpu": {
				{1705320000.0, "42.5"},
				{1705320060.0, "55.3"},
				{1705320120.0, "38.1"},
			},
			"disk.0.iops.read": {
				{1705320000.0, "100"},
				{1705320060.0, "150"},
			},
			"disk.0.iops.write": {
				{1705320000.0, "50"},
				{1705320060.0, "75"},
			},
			"network.0.bandwidth.in": {
				{1705320000.0, "1024"},
				{1705320060.0, "2048"},
			},
			"network.0.bandwidth.out": {
				{1705320000.0, "512"},
				{1705320060.0, "768"},
			},
		},
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/servers/42/metrics" {
			t.Errorf("expected path /servers/42/metrics, got %s", r.URL.Path)
		}

		// Verify query params include all three metric types.
		types := r.URL.Query()["type"]
		wantTypes := map[string]bool{"cpu": false, "disk": false, "network": false}
		for _, typ := range types {
			if _, ok := wantTypes[typ]; ok {
				wantTypes[typ] = true
			}
		}
		for typ, found := range wantTypes {
			if !found {
				t.Errorf("expected metric type %q in query, got types: %v", typ, types)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(srv.Close)

	provider := newTestHetznerProvider(t, srv.URL, "test-token")
	ctx := context.Background()

	start := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 15, 13, 0, 0, 0, time.UTC)

	metrics, err := provider.GetServerMetrics(ctx, "42", []domain.MetricType{
		domain.MetricCPU,
		domain.MetricDisk,
		domain.MetricNetwork,
	}, start, end)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify CPU series.
	cpuSeries, ok := metrics.TimeSeries["cpu"]
	if !ok {
		t.Fatal("expected 'cpu' time series")
	}
	wantCPU := []domain.MetricsPoint{
		{Timestamp: 1705320000.0, Value: 42.5},
		{Timestamp: 1705320060.0, Value: 55.3},
		{Timestamp: 1705320120.0, Value: 38.1},
	}
	if diff := cmp.Diff(wantCPU, cpuSeries.Values); diff != "" {
		t.Errorf("cpu values mismatch (-want +got):\n%s", diff)
	}

	// Verify disk read series.
	diskRead, ok := metrics.TimeSeries["disk.0.iops.read"]
	if !ok {
		t.Fatal("expected 'disk.0.iops.read' time series")
	}
	wantDiskRead := []domain.MetricsPoint{
		{Timestamp: 1705320000.0, Value: 100},
		{Timestamp: 1705320060.0, Value: 150},
	}
	if diff := cmp.Diff(wantDiskRead, diskRead.Values); diff != "" {
		t.Errorf("disk read values mismatch (-want +got):\n%s", diff)
	}

	// Verify network bandwidth in series.
	netIn, ok := metrics.TimeSeries["network.0.bandwidth.in"]
	if !ok {
		t.Fatal("expected 'network.0.bandwidth.in' time series")
	}
	wantNetIn := []domain.MetricsPoint{
		{Timestamp: 1705320000.0, Value: 1024},
		{Timestamp: 1705320060.0, Value: 2048},
	}
	if diff := cmp.Diff(wantNetIn, netIn.Values); diff != "" {
		t.Errorf("network bandwidth in values mismatch (-want +got):\n%s", diff)
	}

	// Verify all expected series are present.
	expectedKeys := []string{
		"cpu", "disk.0.iops.read", "disk.0.iops.write",
		"network.0.bandwidth.in", "network.0.bandwidth.out",
	}
	for _, key := range expectedKeys {
		if _, ok := metrics.TimeSeries[key]; !ok {
			t.Errorf("expected time series %q, not found", key)
		}
	}
}

func TestGetServerMetrics_InvalidID(t *testing.T) {
	srv := newTestAPI(t, map[string]any{})
	provider := newTestHetznerProvider(t, srv.URL, "test-token")

	_, err := provider.GetServerMetrics(
		context.Background(),
		"not-a-number",
		[]domain.MetricType{domain.MetricCPU},
		time.Now().Add(-time.Hour),
		time.Now(),
	)
	if err == nil {
		t.Fatal("expected error for invalid server ID")
	}
	if !containsString(err.Error(), "invalid server ID") {
		t.Errorf("expected error to mention 'invalid server ID', got: %v", err)
	}
}

func TestGetServerMetrics_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "not_found",
				"message": "server not found",
			},
		})
	}))
	t.Cleanup(srv.Close)

	provider := newTestHetznerProvider(t, srv.URL, "test-token")

	_, err := provider.GetServerMetrics(
		context.Background(),
		"99999",
		[]domain.MetricType{domain.MetricCPU},
		time.Now().Add(-time.Hour),
		time.Now(),
	)
	if err == nil {
		t.Fatal("expected error for non-existent server")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestGetServerMetrics_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "unauthorized",
				"message": "unable to authenticate",
			},
		})
	}))
	t.Cleanup(srv.Close)

	provider := newTestHetznerProvider(t, srv.URL, "bad-token")

	_, err := provider.GetServerMetrics(
		context.Background(),
		"42",
		[]domain.MetricType{domain.MetricCPU},
		time.Now().Add(-time.Hour),
		time.Now(),
	)
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

func TestGetServerMetrics_EmptySeries(t *testing.T) {
	response := testMetricsResponse(
		"2024-01-15T12:00:00Z",
		"2024-01-15T13:00:00Z",
		60,
		map[string][][2]any{
			"cpu": {},
		},
	)

	srv := newTestAPI(t, response)
	provider := newTestHetznerProvider(t, srv.URL, "test-token")

	metrics, err := provider.GetServerMetrics(
		context.Background(),
		"42",
		[]domain.MetricType{domain.MetricCPU},
		time.Now().Add(-time.Hour),
		time.Now(),
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	cpuSeries, ok := metrics.TimeSeries["cpu"]
	if !ok {
		t.Fatal("expected 'cpu' time series")
	}
	if len(cpuSeries.Values) != 0 {
		t.Errorf("expected empty values, got %d", len(cpuSeries.Values))
	}
}

// containsString is a small helper for substring matching in test assertions.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstr(s, substr)
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
