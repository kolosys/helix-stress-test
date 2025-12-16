package metrics

import (
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics collects and aggregates performance metrics.
type Metrics struct {
	mu sync.RWMutex

	// Request metrics
	totalRequests    atomic.Int64
	successRequests  atomic.Int64
	errorRequests    atomic.Int64
	latencies        []time.Duration
	latenciesMu      sync.Mutex

	// Error tracking
	errorsByStatus map[int]int64
	errorsMu       sync.Mutex

	// Throughput
	startTime    time.Time
	lastSecond   time.Time
	requestsThisSecond atomic.Int64
	currentRPS   atomic.Int64

	// Memory metrics
	initialMemStats runtime.MemStats
	memStats        runtime.MemStats
	memStatsMu      sync.Mutex
}

// New creates a new Metrics collector.
func New() *Metrics {
	m := &Metrics{
		latencies:      make([]time.Duration, 0, 10000),
		errorsByStatus: make(map[int]int64),
		startTime:      time.Now(),
		lastSecond:     time.Now(),
	}

	runtime.ReadMemStats(&m.initialMemStats)
	return m
}

// RecordRequest records a request with its latency and status code.
func (m *Metrics) RecordRequest(latency time.Duration, statusCode int) {
	m.totalRequests.Add(1)

	if statusCode >= 200 && statusCode < 400 {
		m.successRequests.Add(1)
	} else {
		m.errorRequests.Add(1)
		m.errorsMu.Lock()
		m.errorsByStatus[statusCode]++
		m.errorsMu.Unlock()
	}

	m.latenciesMu.Lock()
	m.latencies = append(m.latencies, latency)
	m.latenciesMu.Unlock()

	// Update RPS calculation
	now := time.Now()
	if now.Sub(m.lastSecond) >= time.Second {
		m.currentRPS.Store(m.requestsThisSecond.Load())
		m.requestsThisSecond.Store(0)
		m.lastSecond = now
	} else {
		m.requestsThisSecond.Add(1)
	}
}

// RecordError records an error response.
func (m *Metrics) RecordError(statusCode int) {
	m.errorRequests.Add(1)
	m.errorsMu.Lock()
	m.errorsByStatus[statusCode]++
	m.errorsMu.Unlock()
}

// Snapshot captures a snapshot of current metrics.
type Snapshot struct {
	StartTime         time.Time
	EndTime           time.Time
	Duration          time.Duration
	TotalRequests     int64
	SuccessRequests   int64
	ErrorRequests     int64
	CurrentRPS        int64
	AverageRPS        float64
	LatencyP50        time.Duration
	LatencyP95        time.Duration
	LatencyP99        time.Duration
	LatencyP999       time.Duration
	LatencyMin        time.Duration
	LatencyMax        time.Duration
	LatencyMean       time.Duration
	ErrorsByStatus    map[int]int64
	ErrorRate         float64
	MemoryAllocated   uint64
	MemoryTotalAlloc  uint64
	MemorySys         uint64
	NumGC             uint32
	GCPercent         float64
}

// Snapshot captures the current state of metrics.
func (m *Metrics) Snapshot() Snapshot {
	m.memStatsMu.Lock()
	runtime.ReadMemStats(&m.memStats)
	memStats := m.memStats
	m.memStatsMu.Unlock()

	m.latenciesMu.Lock()
	latencies := make([]time.Duration, len(m.latencies))
	copy(latencies, m.latencies)
	m.latenciesMu.Unlock()

	m.errorsMu.Lock()
	errorsByStatus := make(map[int]int64)
	for k, v := range m.errorsByStatus {
		errorsByStatus[k] = v
	}
	m.errorsMu.Unlock()

	total := m.totalRequests.Load()
	success := m.successRequests.Load()
	errors := m.errorRequests.Load()
	now := time.Now()
	duration := now.Sub(m.startTime)

	var (
		latencyP50, latencyP95, latencyP99, latencyP999 time.Duration
		latencyMin, latencyMax, latencyMean              time.Duration
	)

	if len(latencies) > 0 {
		sorted := make([]time.Duration, len(latencies))
		copy(sorted, latencies)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i] < sorted[j]
		})

		latencyMin = sorted[0]
		latencyMax = sorted[len(sorted)-1]

		var sum time.Duration
		for _, l := range sorted {
			sum += l
		}
		latencyMean = sum / time.Duration(len(sorted))

		latencyP50 = percentile(sorted, 0.50)
		latencyP95 = percentile(sorted, 0.95)
		latencyP99 = percentile(sorted, 0.99)
		latencyP999 = percentile(sorted, 0.999)
	}

	var errorRate float64
	if total > 0 {
		errorRate = float64(errors) / float64(total) * 100
	}

	var avgRPS float64
	if duration > 0 {
		avgRPS = float64(total) / duration.Seconds()
	}

	return Snapshot{
		StartTime:        m.startTime,
		EndTime:          now,
		Duration:         duration,
		TotalRequests:    total,
		SuccessRequests:  success,
		ErrorRequests:    errors,
		CurrentRPS:       m.currentRPS.Load(),
		AverageRPS:       avgRPS,
		LatencyP50:       latencyP50,
		LatencyP95:       latencyP95,
		LatencyP99:       latencyP99,
		LatencyP999:      latencyP999,
		LatencyMin:       latencyMin,
		LatencyMax:       latencyMax,
		LatencyMean:      latencyMean,
		ErrorsByStatus:   errorsByStatus,
		ErrorRate:        errorRate,
		MemoryAllocated:  memStats.Alloc - m.initialMemStats.Alloc,
		MemoryTotalAlloc: memStats.TotalAlloc - m.initialMemStats.TotalAlloc,
		MemorySys:        memStats.Sys - m.initialMemStats.Sys,
		NumGC:            memStats.NumGC - m.initialMemStats.NumGC,
		GCPercent:        float64(memStats.NumGC-m.initialMemStats.NumGC) / duration.Seconds() * 60,
	}
}

// percentile calculates the percentile value from a sorted slice.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(len(sorted)) * p)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// Reset clears all metrics.
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalRequests.Store(0)
	m.successRequests.Store(0)
	m.errorRequests.Store(0)

	m.latenciesMu.Lock()
	m.latencies = m.latencies[:0]
	m.latenciesMu.Unlock()

	m.errorsMu.Lock()
	m.errorsByStatus = make(map[int]int64)
	m.errorsMu.Unlock()

	m.startTime = time.Now()
	m.lastSecond = time.Now()
	m.requestsThisSecond.Store(0)
	m.currentRPS.Store(0)

	runtime.ReadMemStats(&m.initialMemStats)
}

