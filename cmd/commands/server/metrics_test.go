package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/server/providers"
	"nathanbeddoewebdev/vpsm/internal/services/auth"
)

type metricsMockProvider struct {
	displayName string
	metrics     *domain.ServerMetrics
	metricsErr  error
	gotID       string
	gotTypes    []domain.MetricType
}

func (m *metricsMockProvider) GetDisplayName() string { return m.displayName }
func (m *metricsMockProvider) CreateServer(_ context.Context, _ domain.CreateServerOpts) (*domain.Server, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *metricsMockProvider) DeleteServer(_ context.Context, _ string) error {
	return fmt.Errorf("not implemented")
}
func (m *metricsMockProvider) GetServer(_ context.Context, _ string) (*domain.Server, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *metricsMockProvider) ListServers(_ context.Context) ([]domain.Server, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *metricsMockProvider) StartServer(_ context.Context, _ string) (*domain.ActionStatus, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *metricsMockProvider) StopServer(_ context.Context, _ string) (*domain.ActionStatus, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *metricsMockProvider) GetServerMetrics(_ context.Context, id string, types []domain.MetricType, _, _ time.Time) (*domain.ServerMetrics, error) {
	m.gotID = id
	m.gotTypes = types
	return m.metrics, m.metricsErr
}

func registerMetricsMockProvider(t *testing.T, name string, mock *metricsMockProvider) {
	t.Helper()
	providers.Reset()
	t.Cleanup(func() { providers.Reset() })
	providers.Register(name, func(store auth.Store) (domain.Provider, error) {
		return mock, nil
	})
}

func execMetrics(t *testing.T, providerName string, extraArgs ...string) (stdout, stderr string) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := NewCommand()
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	args := append([]string{"metrics", "--provider", providerName}, extraArgs...)
	cmd.SetArgs(args)
	cmd.Execute()
	return outBuf.String(), errBuf.String()
}

func TestMetricsCommand_TableOutput(t *testing.T) {
	now := time.Now()
	metrics := &domain.ServerMetrics{
		Start: now.Add(-1 * time.Hour),
		End:   now,
		Step:  60,
		TimeSeries: map[string]domain.MetricsTimeSeries{
			"cpu": {
				Name: "cpu",
				Values: []domain.MetricsPoint{
					{Timestamp: 1, Value: 1.2},
					{Timestamp: 2, Value: 0.5},
					{Timestamp: 3, Value: 3.8},
				},
			},
			"disk.0.iops.read": {
				Name: "disk.0.iops.read",
				Values: []domain.MetricsPoint{
					{Timestamp: 1, Value: 0},
					{Timestamp: 2, Value: 0},
					{Timestamp: 3, Value: 0},
				},
			},
			"disk.0.iops.write": {
				Name: "disk.0.iops.write",
				Values: []domain.MetricsPoint{
					{Timestamp: 1, Value: 0.7},
					{Timestamp: 2, Value: 5.2},
					{Timestamp: 3, Value: 7.4},
				},
			},
			"network.0.bandwidth.in": {
				Name: "network.0.bandwidth.in",
				Values: []domain.MetricsPoint{
					{Timestamp: 1, Value: 4.3},
					{Timestamp: 2, Value: 100},
					{Timestamp: 3, Value: 836.8},
				},
			},
			"network.0.bandwidth.out": {
				Name: "network.0.bandwidth.out",
				Values: []domain.MetricsPoint{
					{Timestamp: 1, Value: 0},
					{Timestamp: 2, Value: 1600},
					{Timestamp: 3, Value: 975.1},
				},
			},
		},
	}

	mock := &metricsMockProvider{
		displayName: "Mock",
		metrics:     metrics,
	}
	registerMetricsMockProvider(t, "mock", mock)

	stdout, _ := execMetrics(t, "mock", "--id", "42")

	if mock.gotID != "42" {
		t.Errorf("expected GetServerMetrics called with ID '42', got %q", mock.gotID)
	}

	assertContainsAll(t, stdout, "stdout", []string{
		"cpu", "3.8%", "0.5%", "3.8%",
		"disk.0.iops.write", "7.4",
		"network.0.bandwidth.in", "836.8B/s",
		"network.0.bandwidth.out", "975.1B/s", "1.6KB/s",
	})
}

func TestMetricsCommand_JSONOutput(t *testing.T) {
	now := time.Now()
	metrics := &domain.ServerMetrics{
		Start: now.Add(-1 * time.Hour),
		End:   now,
		Step:  60,
		TimeSeries: map[string]domain.MetricsTimeSeries{
			"cpu": {
				Name: "cpu",
				Values: []domain.MetricsPoint{
					{Timestamp: 1, Value: 1.5},
				},
			},
		},
	}

	mock := &metricsMockProvider{
		displayName: "Mock",
		metrics:     metrics,
	}
	registerMetricsMockProvider(t, "mock", mock)

	stdout, _ := execMetrics(t, "mock", "--id", "42", "-o", "json")

	var got domain.ServerMetrics
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput:\n%s", err, stdout)
	}

	if len(got.TimeSeries) != 1 {
		t.Errorf("expected 1 time series, got %d", len(got.TimeSeries))
	}
	if ts, ok := got.TimeSeries["cpu"]; !ok {
		t.Error("expected 'cpu' time series")
	} else if len(ts.Values) != 1 {
		t.Errorf("expected 1 value in cpu series, got %d", len(ts.Values))
	}
}

func TestMetricsCommand_FetchError(t *testing.T) {
	mock := &metricsMockProvider{
		displayName: "Mock",
		metricsErr:  fmt.Errorf("server not found"),
	}
	registerMetricsMockProvider(t, "mock", mock)

	stdout, stderr := execMetrics(t, "mock", "--id", "999")

	if !strings.Contains(stderr, "server not found") {
		t.Errorf("expected 'server not found' on stderr, got:\n%s", stderr)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout on error, got:\n%s", stdout)
	}
}

func TestMetricsCommand_UnknownProvider(t *testing.T) {
	providers.Reset()
	t.Cleanup(func() { providers.Reset() })

	_, stderr := execMetrics(t, "nonexistent", "--id", "42")

	if !strings.Contains(stderr, "unknown provider") {
		t.Errorf("expected 'unknown provider' error on stderr, got:\n%s", stderr)
	}
}

func TestMetricsCommand_RequestsAllMetricTypes(t *testing.T) {
	mock := &metricsMockProvider{
		displayName: "Mock",
		metrics:     &domain.ServerMetrics{TimeSeries: make(map[string]domain.MetricsTimeSeries)},
	}
	registerMetricsMockProvider(t, "mock", mock)

	_, _ = execMetrics(t, "mock", "--id", "42")

	expectedTypes := []domain.MetricType{domain.MetricCPU, domain.MetricDisk, domain.MetricNetwork}
	if len(mock.gotTypes) != len(expectedTypes) {
		t.Errorf("expected %d metric types, got %d", len(expectedTypes), len(mock.gotTypes))
	}
	for _, want := range expectedTypes {
		found := slices.Contains(mock.gotTypes, want)
		if !found {
			t.Errorf("expected metric type %q in request", want)
		}
	}
}
