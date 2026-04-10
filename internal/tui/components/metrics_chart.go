package components

import (
	"fmt"
	"math"

	"nathanbeddoewebdev/vpsm/internal/tui/styles"

	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	slc "github.com/NimbleMarkets/ntcharts/linechart/streamlinechart"
	"github.com/charmbracelet/lipgloss"
)

// chartHeight is the total height for single-series charts (including axes).
const chartHeight = 8

// dualChartHeight is the total height for dual-series charts.
const dualChartHeight = 8

// ySteps controls the number of labeled ticks on the Y axis.
const ySteps = 3

// xSteps controls the spacing of labeled ticks on the X axis.
const xSteps = 4

// DualChartColors holds the color pair for a dual-series chart.
type DualChartColors struct {
	Color1 lipgloss.AdaptiveColor
	Color2 lipgloss.AdaptiveColor
}

// --- Formatters ---

// timeXFormatter returns an XLabelFormatter that maps X values (in minutes)
// to human-readable time labels like "-60m", "-30m", "now".
func timeXFormatter() func(int, float64) string {
	return func(_ int, v float64) string {
		m := int(math.Round(v))
		if m == 0 {
			return "now"
		}
		return fmt.Sprintf("%dm", m)
	}
}

// yFormatter returns a YLabelFormatter for axis tick labels.
func yFormatter(suffix string) func(int, float64) string {
	return func(_ int, v float64) string {
		return formatCompact(v, suffix)
	}
}

// formatCompact renders a value for axis labels — minimal decimals, omit trailing zeros.
func formatCompact(v float64, suffix string) string {
	switch {
	case v >= 1_000_000_000:
		return trimTrailingZero(fmt.Sprintf("%.1f", v/1_000_000_000)) + "G" + suffix
	case v >= 1_000_000:
		return trimTrailingZero(fmt.Sprintf("%.1f", v/1_000_000)) + "M" + suffix
	case v >= 1_000:
		return trimTrailingZero(fmt.Sprintf("%.1f", v/1_000)) + "K" + suffix
	default:
		if v == math.Trunc(v) {
			return fmt.Sprintf("%d%s", int(v), suffix)
		}
		return trimTrailingZero(fmt.Sprintf("%.1f", v)) + suffix
	}
}

// formatSummary renders a value for summary lines.
func formatSummary(v float64, suffix string) string {
	switch {
	case v >= 1_000_000_000:
		return fmt.Sprintf("%.1fG%s", v/1_000_000_000, suffix)
	case v >= 1_000_000:
		return fmt.Sprintf("%.1fM%s", v/1_000_000, suffix)
	case v >= 1_000:
		return fmt.Sprintf("%.1fK%s", v/1_000, suffix)
	case v == 0:
		return "0" + suffix
	default:
		return trimTrailingZero(fmt.Sprintf("%.1f", v)) + suffix
	}
}

// trimTrailingZero removes ".0" from formatted numbers (e.g., "3.0" -> "3").
func trimTrailingZero(s string) string {
	if len(s) >= 2 && s[len(s)-2:] == ".0" {
		return s[:len(s)-2]
	}
	return s
}

// renderSummary builds a summary line with muted labels and white values.
// e.g. "  cur: 1.2%  min: 0.5%  max: 3.8%"
func renderSummary(cur, min, max float64, suffix string) string {
	muted := styles.MutedText
	val := styles.Value
	return "  " +
		muted.Render("cur: ") + val.Render(formatSummary(cur, suffix)) + "  " +
		muted.Render("min: ") + val.Render(formatSummary(min, suffix)) + "  " +
		muted.Render("max: ") + val.Render(formatSummary(max, suffix))
}

// renderLegendSummary builds a summary line with a colored legend prefix.
func renderLegendSummary(legend string, legendStyle lipgloss.Style, cur, min, max float64, suffix string) string {
	muted := styles.MutedText
	val := styles.Value
	return "  " + legendStyle.Render(legend) + "  " +
		muted.Render("cur: ") + val.Render(formatSummary(cur, suffix)) + "  " +
		muted.Render("min: ") + val.Render(formatSummary(min, suffix)) + "  " +
		muted.Render("max: ") + val.Render(formatSummary(max, suffix))
}

// padDataLeft prepends zeros so that len(data) >= graphWidth,
// filling the entire chart area from left to right.
func padDataLeft(data []float64, graphWidth int) []float64 {
	if len(data) >= graphWidth {
		return data
	}
	padded := make([]float64, graphWidth)
	offset := graphWidth - len(data)
	copy(padded[offset:], data)
	return padded
}

// --- Chart constructors ---

// newChart creates a single-series streamlinechart with axes and data.
func newChart(width, height int, data []float64, suffix string, lineStyle lipgloss.Style) slc.Model {
	_, maxVal := minMax(data)
	if maxVal == 0 {
		maxVal = 1
	}
	yMax := maxVal * 1.15

	axisStyle := lipgloss.NewStyle().Foreground(styles.DimGray)
	labelStyle := lipgloss.NewStyle().Foreground(styles.Muted)

	chart := slc.New(width, height,
		slc.WithYRange(0, yMax),
		slc.WithXRange(-60, 0),
		slc.WithXYSteps(xSteps, ySteps),
		slc.WithStyles(runes.ArcLineStyle, lineStyle),
		slc.WithAxesStyles(axisStyle, labelStyle),
	)
	chart.YLabelFormatter = yFormatter(suffix)
	chart.XLabelFormatter = timeXFormatter()
	chart.UpdateGraphSizes()

	// Pad data to fill the full graphing area so the line spans left to right.
	gw := chart.GraphWidth()
	padded := padDataLeft(data, gw)
	for _, v := range padded {
		chart.Push(v)
	}

	chart.Draw()
	return chart
}

// newDualChart creates a streamlinechart with two named datasets.
// Series1 uses ArcLineStyle, series2 uses ThinLineStyle for visual distinction.
func newDualChart(width, height int, s1, s2 []float64, name1, name2 string, suffix string, style1, style2 lipgloss.Style) slc.Model {
	_, max1 := minMax(s1)
	_, max2 := minMax(s2)
	maxVal := math.Max(max1, max2)
	if maxVal == 0 {
		maxVal = 1
	}
	yMax := maxVal * 1.15

	axisStyle := lipgloss.NewStyle().Foreground(styles.DimGray)
	labelStyle := lipgloss.NewStyle().Foreground(styles.Muted)

	chart := slc.New(width, height,
		slc.WithYRange(0, yMax),
		slc.WithXRange(-60, 0),
		slc.WithXYSteps(xSteps, ySteps),
		slc.WithStyles(runes.ArcLineStyle, style1),
		slc.WithAxesStyles(axisStyle, labelStyle),
		slc.WithDataSetStyles(name1, runes.ArcLineStyle, style1),
		slc.WithDataSetStyles(name2, runes.ThinLineStyle, style2),
	)
	chart.YLabelFormatter = yFormatter(suffix)
	chart.XLabelFormatter = timeXFormatter()
	chart.UpdateGraphSizes()

	// Pad both series to fill the full graphing area.
	gw := chart.GraphWidth()
	padded1 := padDataLeft(s1, gw)
	padded2 := padDataLeft(s2, gw)
	for _, v := range padded1 {
		chart.PushDataSet(name1, v)
	}
	for _, v := range padded2 {
		chart.PushDataSet(name2, v)
	}

	chart.DrawAll()
	return chart
}

// --- Public chart renderers ---

// MetricsChart renders a single-series line chart with axes and a label header.
func MetricsChart(label string, data []float64, width int, suffix string) string {
	if len(data) == 0 {
		return styles.MutedText.Render(label + ": no data")
	}

	chartWidth := max(width, 20)

	lineStyle := lipgloss.NewStyle().Foreground(styles.Blue)
	chart := newChart(chartWidth, chartHeight, data, suffix, lineStyle)

	current := data[len(data)-1]
	min, max := minMax(data)
	summary := renderSummary(current, min, max, suffix)

	header := styles.Label.Render(label)
	return lipgloss.JoinVertical(lipgloss.Left, header, chart.View(), summary)
}

// MetricsDualChart renders two overlaid series on a single chart with shared
// axes, per-series legends, and summaries. Colors are specified by the caller.
func MetricsDualChart(label string, series1, series2 []float64, legend1, legend2 string, width int, suffix string, colors DualChartColors) string {
	if len(series1) == 0 && len(series2) == 0 {
		return styles.MutedText.Render(label + ": no data")
	}

	chartWidth := max(width, 20)

	// Capture original emptiness before filling with zeros for chart rendering.
	orig1Empty := len(series1) == 0
	orig2Empty := len(series2) == 0

	// Ensure both series are present for the chart; use empty slice as fallback.
	if orig1Empty {
		series1 = make([]float64, len(series2))
	}
	if orig2Empty {
		series2 = make([]float64, len(series1))
	}

	style1 := lipgloss.NewStyle().Foreground(colors.Color1)
	style2 := lipgloss.NewStyle().Foreground(colors.Color2)
	chart := newDualChart(chartWidth, dualChartHeight, series1, series2, legend1, legend2, suffix, style1, style2)

	// Per-series summary lines with colored legend labels.
	legendStyle1 := lipgloss.NewStyle().Foreground(colors.Color1).Bold(true)
	legendStyle2 := lipgloss.NewStyle().Foreground(colors.Color2).Bold(true)

	var summaryParts []string
	if !orig1Empty {
		cur1 := series1[len(series1)-1]
		min1, max1 := minMax(series1)
		summaryParts = append(summaryParts,
			renderLegendSummary(legend1, legendStyle1, cur1, min1, max1, suffix),
		)
	}
	if !orig2Empty {
		cur2 := series2[len(series2)-1]
		min2, max2 := minMax(series2)
		summaryParts = append(summaryParts,
			renderLegendSummary(legend2, legendStyle2, cur2, min2, max2, suffix),
		)
	}

	header := styles.Label.Render(label)
	sections := []string{header, chart.View()}
	sections = append(sections, summaryParts...)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// --- Helpers ---

// minMax returns the minimum and maximum values from a slice.
func minMax(data []float64) (float64, float64) {
	if len(data) == 0 {
		return 0, 0
	}
	min, max := data[0], data[0]
	for _, v := range data[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}
