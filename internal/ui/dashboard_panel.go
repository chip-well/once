package ui

import (
	"fmt"
	"image/color"
	"regexp"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/metrics"
)

type DashboardPanel struct {
	app           *docker.Application
	scraper       *metrics.MetricsScraper
	dockerScraper *docker.Scraper
	cpuChart      Chart
	memoryChart   Chart
	requestChart  Chart
	errorChart    Chart
}

func NewDashboardPanel(app *docker.Application, scraper *metrics.MetricsScraper, dockerScraper *docker.Scraper) DashboardPanel {
	return DashboardPanel{
		app:           app,
		scraper:       scraper,
		dockerScraper: dockerScraper,
		cpuChart:      NewChart("CPU %", ChartColors.CPU, UnitPercent),
		memoryChart:   NewChart("Memory", ChartColors.Memory, UnitBytes),
		requestChart:  NewChart("Req/min", ChartColors.Requests, UnitCount),
		errorChart:    NewChart("Err/min", ChartColors.Errors, UnitCount),
	}
}

func (p DashboardPanel) View(selected bool, toggling bool, width int) string {
	title := p.app.Settings.URL()
	if title == "" {
		title = p.app.Settings.Name
	}

	stateLine := renderStateLine(p.app, toggling)

	innerWidth := width - 3 // 1 indicator + 1 left pad + 1 right pad
	if innerWidth < 0 {
		innerWidth = 0
	}

	titleLine := Styles.Title.Render(title)

	var lines []string
	lines = append(lines, titleLine)
	lines = append(lines, stateLine)

	// Show charts when the app is running and there's enough width
	chartHeight := 6
	minChartWidth := 10
	if p.app.Running && innerWidth >= minChartWidth*4+3 {
		chartWidth := (innerWidth - 3) / 4 // 3 single-char gaps between 4 charts

		cpuChart := p.cpuChart.View(p.fetchCPUData(), chartWidth, chartHeight)
		memChart := p.memoryChart.View(p.fetchMemoryData(), chartWidth, chartHeight)
		reqChart := p.requestChart.View(p.fetchRequestData(), chartWidth, chartHeight)
		errChart := p.errorChart.View(p.fetchErrorData(), chartWidth, chartHeight)

		chartsRow := lipgloss.JoinHorizontal(lipgloss.Top, cpuChart, " ", memChart, " ", reqChart, " ", errChart)
		lines = append(lines, chartsRow)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)

	body := lipgloss.NewStyle().
		Background(Colors.PanelBg).
		Width(width - 1).
		Padding(0, 1).
		Height(PanelHeight).
		Render(content)
	body = fixBackground(body, Colors.PanelBg)

	indicator := p.renderIndicator(selected)

	return lipgloss.JoinHorizontal(lipgloss.Top, indicator, body)
}

// Private

func (p DashboardPanel) renderIndicator(selected bool) string {
	rows := make([]string, PanelHeight)
	if selected {
		line := lipgloss.NewStyle().Foreground(Colors.Focused).Render("▐")
		for i := range rows {
			rows[i] = line
		}
	} else {
		for i := range rows {
			rows[i] = " "
		}
	}
	return strings.Join(rows, "\n")
}

func (p DashboardPanel) fetchCPUData() []float64 {
	samples := p.dockerScraper.Fetch(p.app.Settings.Name, ChartHistoryLength)
	data := make([]float64, len(samples))
	for i, s := range samples {
		data[i] = s.CPUPercent
	}
	slices.Reverse(data)
	return data
}

func (p DashboardPanel) fetchMemoryData() []float64 {
	samples := p.dockerScraper.Fetch(p.app.Settings.Name, ChartHistoryLength)
	data := make([]float64, len(samples))
	for i, s := range samples {
		data[i] = float64(s.MemoryBytes)
	}
	slices.Reverse(data)
	return data
}

func (p DashboardPanel) fetchRequestData() []float64 {
	samples := p.scraper.Fetch(p.app.Settings.Name, ChartHistoryLength)
	data := make([]float64, len(samples))
	for i, s := range samples {
		data[i] = float64(s.Success + s.ClientErrors + s.ServerErrors)
	}
	slices.Reverse(data)
	return SlidingSum(data, ChartSlidingWindow)
}

func (p DashboardPanel) fetchErrorData() []float64 {
	samples := p.scraper.Fetch(p.app.Settings.Name, ChartHistoryLength)
	data := make([]float64, len(samples))
	for i, s := range samples {
		data[i] = float64(s.ServerErrors)
	}
	slices.Reverse(data)
	return SlidingSum(data, ChartSlidingWindow)
}

// Helpers

func renderStateLine(app *docker.Application, toggling bool) string {
	var status string
	var statusColor color.Color
	if toggling && app.Running {
		status = "stopping..."
		statusColor = Colors.Warning
	} else if toggling {
		status = "starting..."
		statusColor = Colors.Warning
	} else if app.Running {
		status = "running"
		statusColor = Colors.Success
	} else {
		status = "stopped"
		statusColor = Colors.Error
	}

	stateStyle := lipgloss.NewStyle().Foreground(statusColor)
	stateDisplay := fmt.Sprintf("State: %s", stateStyle.Render(status))

	if app.Running && !app.RunningSince.IsZero() {
		stateDisplay += fmt.Sprintf(" (up %s)", formatDuration(time.Since(app.RunningSince)))
	}

	return stateDisplay
}

// fixBackground works around a lipgloss limitation where inner styled elements
// emit ANSI reset sequences that clear the outer background color. This
// replaces every reset with reset + re-apply, so the background persists
// across styled spans within each line.
var ansiReset = regexp.MustCompile(`\x1b\[0?m`)

func fixBackground(s string, bg color.Color) string {
	r, g, b, _ := bg.RGBA()
	bgSeq := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
	fixed := ansiReset.ReplaceAllString(s, "${0}"+bgSeq)
	// The replace above also patches the trailing reset on each line, which
	// would cause the background to bleed past the panel. Strip those so
	// each line ends with a clean reset.
	lines := strings.Split(fixed, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, bgSeq)
	}
	return strings.Join(lines, "\n")
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		if mins == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, hours)
}
