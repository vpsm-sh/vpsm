package providers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"nathanbeddoewebdev/vpsm/internal/server/domain"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// --- MetricsProvider implementation ---

// GetServerMetrics fetches time-series metrics for a server over the given
// time range. The step is calculated automatically based on the duration
// to produce approximately 60 data points.
func (h *HetznerProvider) GetServerMetrics(ctx context.Context, serverID string, types []domain.MetricType, start, end time.Time) (*domain.ServerMetrics, error) {
	hcloudTypes := make([]hcloud.ServerMetricType, 0, len(types))
	for _, t := range types {
		switch t {
		case domain.MetricCPU:
			hcloudTypes = append(hcloudTypes, hcloud.ServerMetricCPU)
		case domain.MetricDisk:
			hcloudTypes = append(hcloudTypes, hcloud.ServerMetricDisk)
		case domain.MetricNetwork:
			hcloudTypes = append(hcloudTypes, hcloud.ServerMetricNetwork)
		default:
			return nil, fmt.Errorf("unsupported metric type: %q", t)
		}
	}

	// Calculate step to produce ~60 data points.
	duration := end.Sub(start)
	step := max(int(duration.Seconds()/60), 1)

	opts := hcloud.ServerGetMetricsOpts{
		Types: hcloudTypes,
		Start: start,
		End:   end,
		Step:  step,
	}

	hzMetrics, err := h.hcloudService.GetServerMetrics(ctx, serverID, opts)
	if err != nil {
		if hcloud.IsError(err, hcloud.ErrorCodeNotFound) {
			return nil, fmt.Errorf("failed to get server metrics: %w", domain.ErrNotFound)
		}
		if hcloud.IsError(err, hcloud.ErrorCodeUnauthorized) {
			return nil, fmt.Errorf("failed to get server metrics: %w", domain.ErrUnauthorized)
		}
		if hcloud.IsError(err, hcloud.ErrorCodeRateLimitExceeded) {
			return nil, fmt.Errorf("failed to get server metrics: %w", domain.ErrRateLimited)
		}
		return nil, fmt.Errorf("failed to get server metrics: %w", err)
	}

	return toDomainMetrics(hzMetrics), nil
}

// toDomainMetrics converts hcloud.ServerMetrics to domain.ServerMetrics.
// Values that cannot be parsed as float64 are silently skipped.
func toDomainMetrics(hz *hcloud.ServerMetrics) *domain.ServerMetrics {
	if hz == nil {
		return &domain.ServerMetrics{
			TimeSeries: make(map[string]domain.MetricsTimeSeries),
		}
	}

	ts := make(map[string]domain.MetricsTimeSeries, len(hz.TimeSeries))

	for name, values := range hz.TimeSeries {
		points := make([]domain.MetricsPoint, 0, len(values))
		for _, v := range values {
			f, err := strconv.ParseFloat(v.Value, 64)
			if err != nil {
				continue
			}
			points = append(points, domain.MetricsPoint{
				Timestamp: v.Timestamp,
				Value:     f,
			})
		}
		ts[name] = domain.MetricsTimeSeries{
			Name:   name,
			Values: points,
		}
	}

	return &domain.ServerMetrics{
		Start:      hz.Start,
		End:        hz.End,
		Step:       hz.Step,
		TimeSeries: ts,
	}
}
