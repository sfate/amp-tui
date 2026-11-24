package tui

import (
	"fmt"
	"os"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// perfState backs the AMP_TUI_PERF_LOG diagnostics; an empty logPath disables it.
type perfState struct {
	logPath    string
	lastReport time.Time
	msgCounts  map[string]int
}

func perfLogPathFromEnv() string {
	value := strings.TrimSpace(os.Getenv("AMP_TUI_PERF_LOG"))
	if value == "" {
		return ""
	}
	if value == "1" || strings.EqualFold(value, "true") {
		return os.TempDir() + string(os.PathSeparator) + "amp-tui-perf.log"
	}
	return value
}

func (m *Model) recordPerfMsg(msg tea.Msg) {
	if strings.TrimSpace(m.perf.logPath) == "" {
		return
	}
	if m.perf.msgCounts == nil {
		m.perf.msgCounts = map[string]int{}
	}
	m.perf.msgCounts[fmt.Sprintf("%T", msg)]++
	now := time.Now()
	if m.perf.lastReport.IsZero() {
		m.perf.lastReport = now
		return
	}
	if now.Sub(m.perf.lastReport) < 10*time.Second {
		return
	}

	var mem goruntime.MemStats
	goruntime.ReadMemStats(&mem)
	appendPerfLine(m.perf.logPath, fmt.Sprintf(
		"ts=%s goroutines=%d heap_alloc_mb=%.1f queue=%d flow=%t flow_next=%d flow_loading=%t prebuffer_statuses=%d prebuffer_ready=%d prebuffer_ticking=%t msg_counts=%s",
		now.Format(time.RFC3339),
		goruntime.NumGoroutine(),
		float64(mem.Alloc)/(1024*1024),
		len(m.app.QueueTracks),
		m.app.IsFlowQueue,
		m.app.FlowNextIndex,
		m.app.FlowLoadingMore,
		len(m.prebuffer.statuses),
		len(m.prebuffer.ready),
		m.prebuffer.ticking,
		formatPerfCounts(m.perf.msgCounts),
	))
	m.perf.msgCounts = map[string]int{}
	m.perf.lastReport = now
}

func (m Model) recordRenderDuration(duration time.Duration) {
	if strings.TrimSpace(m.perf.logPath) == "" || duration < 25*time.Millisecond {
		return
	}
	appendPerfLine(m.perf.logPath, fmt.Sprintf(
		"ts=%s slow_render_ms=%d width=%d height=%d queue=%d flow=%t visualizer=%t",
		time.Now().Format(time.RFC3339),
		duration.Milliseconds(),
		m.width,
		m.height,
		len(m.app.QueueTracks),
		m.app.IsFlowQueue,
		len(m.visualizer.bands) > 0,
	))
}

func formatPerfCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", key, counts[key]))
	}
	return strings.Join(parts, ",")
}

func appendPerfLine(path string, line string) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = file.WriteString(line + "\n")
}
