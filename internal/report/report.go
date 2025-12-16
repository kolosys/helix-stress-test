package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kolosys/helix-stress-test/internal/config"
	"github.com/kolosys/helix-stress-test/internal/metrics"
)

// Generator generates test reports.
type Generator struct {
	cfg     *config.Config
	metrics *metrics.Metrics
}

// New creates a new report generator.
func New(cfg *config.Config, m *metrics.Metrics) *Generator {
	return &Generator{
		cfg:     cfg,
		metrics: m,
	}
}

// Generate generates and writes the report.
func (g *Generator) Generate() error {
	snapshot := g.metrics.Snapshot()

	var writer io.Writer
	if g.cfg.ReportFile != "" {
		file, err := os.Create(g.cfg.ReportFile)
		if err != nil {
			return fmt.Errorf("failed to create report file: %w", err)
		}
		defer file.Close()
		writer = file
	} else {
		writer = os.Stdout
	}

	switch g.cfg.ReportFormat {
	case "json":
		return g.generateJSON(writer, snapshot)
	case "text":
		return g.generateText(writer, snapshot)
	default:
		return fmt.Errorf("unknown report format: %s", g.cfg.ReportFormat)
	}
}

// generateJSON generates a JSON report.
func (g *Generator) generateJSON(w io.Writer, s metrics.Snapshot) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}

// generateText generates a human-readable text report.
func (g *Generator) generateText(w io.Writer, s metrics.Snapshot) error {
	var b strings.Builder

	b.WriteString("=" + strings.Repeat("=", 78) + "\n")
	b.WriteString("HELIX STRESS TEST REPORT\n")
	b.WriteString("=" + strings.Repeat("=", 78) + "\n\n")

	// Test Timestamps
	b.WriteString("Test Timestamps:\n")
	b.WriteString(strings.Repeat("-", 80) + "\n")
	b.WriteString(fmt.Sprintf("  Start Time:    %s\n", s.StartTime.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("  End Time:      %s\n", s.EndTime.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("  Duration:      %s\n", formatDuration(s.Duration)))
	b.WriteString("\n")

	// Test Configuration
	b.WriteString("Test Configuration:\n")
	b.WriteString(strings.Repeat("-", 80) + "\n")
	b.WriteString(fmt.Sprintf("  Test Type:     %s\n", g.cfg.TestType))
	b.WriteString(fmt.Sprintf("  Server Addr:   %s\n", g.cfg.ServerAddr))
	b.WriteString(fmt.Sprintf("  Concurrent:    %d\n", g.cfg.Concurrent))
	b.WriteString(fmt.Sprintf("  Target RPS:    %d\n", g.cfg.TargetRPS))
	b.WriteString("\n")

	// Request Statistics
	b.WriteString("Request Statistics:\n")
	b.WriteString(strings.Repeat("-", 80) + "\n")
	b.WriteString(fmt.Sprintf("  Total Requests:    %d\n", s.TotalRequests))
	b.WriteString(fmt.Sprintf("  Success Requests:  %d (%.2f%%)\n", s.SuccessRequests, float64(s.SuccessRequests)/float64(s.TotalRequests)*100))
	b.WriteString(fmt.Sprintf("  Error Requests:    %d (%.2f%%)\n", s.ErrorRequests, s.ErrorRate))
	b.WriteString("\n")

	// Throughput
	b.WriteString("Throughput:\n")
	b.WriteString(strings.Repeat("-", 80) + "\n")
	b.WriteString(fmt.Sprintf("  Current RPS:  %d\n", s.CurrentRPS))
	b.WriteString(fmt.Sprintf("  Average RPS:  %.2f\n", s.AverageRPS))
	b.WriteString("\n")

	// Latency
	b.WriteString("Latency:\n")
	b.WriteString(strings.Repeat("-", 80) + "\n")
	if s.LatencyMin > 0 {
		b.WriteString(fmt.Sprintf("  Min:    %s\n", formatDuration(s.LatencyMin)))
		b.WriteString(fmt.Sprintf("  Mean:   %s\n", formatDuration(s.LatencyMean)))
		b.WriteString(fmt.Sprintf("  P50:    %s\n", formatDuration(s.LatencyP50)))
		b.WriteString(fmt.Sprintf("  P95:    %s\n", formatDuration(s.LatencyP95)))
		b.WriteString(fmt.Sprintf("  P99:    %s\n", formatDuration(s.LatencyP99)))
		b.WriteString(fmt.Sprintf("  P99.9:  %s\n", formatDuration(s.LatencyP999)))
		b.WriteString(fmt.Sprintf("  Max:    %s\n", formatDuration(s.LatencyMax)))
	} else {
		b.WriteString("  No latency data available\n")
	}
	b.WriteString("\n")

	// Error Breakdown
	if len(s.ErrorsByStatus) > 0 {
		b.WriteString("Error Breakdown:\n")
		b.WriteString(strings.Repeat("-", 80) + "\n")
		for status, count := range s.ErrorsByStatus {
			b.WriteString(fmt.Sprintf("  %d: %d requests\n", status, count))
		}
		b.WriteString("\n")
	}

	// Memory Statistics
	b.WriteString("Memory Statistics:\n")
	b.WriteString(strings.Repeat("-", 80) + "\n")
	b.WriteString(fmt.Sprintf("  Allocated:     %s\n", formatBytes(s.MemoryAllocated)))
	b.WriteString(fmt.Sprintf("  Total Alloc:   %s\n", formatBytes(s.MemoryTotalAlloc)))
	b.WriteString(fmt.Sprintf("  Sys:           %s\n", formatBytes(s.MemorySys)))
	b.WriteString(fmt.Sprintf("  GC Cycles:     %d\n", s.NumGC))
	b.WriteString(fmt.Sprintf("  GC Rate:       %.2f cycles/min\n", s.GCPercent))
	b.WriteString("\n")

	b.WriteString("=" + strings.Repeat("=", 78) + "\n")

	_, err := w.Write([]byte(b.String()))
	return err
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%.2fns", float64(d.Nanoseconds()))
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.2fµs", float64(d.Nanoseconds())/1000)
	}
	if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Nanoseconds())/1000000)
	}
	return d.String()
}

// formatBytes formats bytes in a human-readable way.
func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// PrintProgress prints real-time progress updates.
func PrintProgress(m *metrics.Metrics, interval time.Duration, done <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// ANSI escape code to clear to end of line
	const clearLine = "\033[K"
	const resetCursor = "\r"

	for {
		select {
		case <-done:
			// Print final newline to move cursor to next line
			fmt.Print("\n")
			return
		case <-ticker.C:
			s := m.Snapshot()
			now := time.Now().Format("15:04:05")
			// Use \r to return to start of line, print progress, clear to end of line
			// This ensures the line stays in place and old content is cleared
			fmt.Printf("%s%s[%s] [%s] Requests: %d | RPS: %.2f | Errors: %d (%.2f%%) | Latency P95: %s",
				resetCursor,
				clearLine,
				now,
				formatDuration(s.Duration),
				s.TotalRequests,
				s.AverageRPS,
				s.ErrorRequests,
				s.ErrorRate,
				formatDuration(s.LatencyP95),
			)
			// Flush output immediately
			os.Stdout.Sync()
		}
	}
}
