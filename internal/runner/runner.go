package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kolosys/helix-stress-test/internal/config"
	"github.com/kolosys/helix-stress-test/internal/metrics"
)

// Endpoint represents a test endpoint.
type Endpoint struct {
	Method string
	Path   string
	Body   string
}

// ParseEndpoint parses an endpoint string (e.g., "GET:/users/123" or "POST:/items").
func ParseEndpoint(s string) (Endpoint, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return Endpoint{}, fmt.Errorf("invalid endpoint format: %s (expected METHOD:PATH)", s)
	}

	method := strings.ToUpper(strings.TrimSpace(parts[0]))
	path := strings.TrimSpace(parts[1])

	// Validate method
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		// Valid
	default:
		return Endpoint{}, fmt.Errorf("invalid HTTP method: %s", method)
	}

	// Generate default body for POST/PUT/PATCH
	var body string
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		body = `{"name":"test","value":"test"}`
	}

	return Endpoint{
		Method: method,
		Path:   path,
		Body:   body,
	}, nil
}

// Runner executes stress tests against a server.
type Runner struct {
	cfg     *config.Config
	client  *http.Client
	metrics *metrics.Metrics
}

// New creates a new Runner.
func New(cfg *config.Config, m *metrics.Metrics) *Runner {
	return &Runner{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        cfg.Concurrent * 2,
				MaxIdleConnsPerHost: cfg.Concurrent,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		metrics: m,
	}
}

// Run executes the stress test based on the configured test type.
func (r *Runner) Run(ctx context.Context) error {
	switch r.cfg.TestType {
	case config.TestTypeLoad:
		return r.runLoadTest(ctx)
	case config.TestTypeSpike:
		return r.runSpikeTest(ctx)
	case config.TestTypeEndurance:
		return r.runEnduranceTest(ctx)
	default:
		return fmt.Errorf("unknown test type: %s", r.cfg.TestType)
	}
}

// runLoadTest runs a sustained load test.
func (r *Runner) runLoadTest(ctx context.Context) error {
	endpoints, err := r.parseEndpoints()
	if err != nil {
		return fmt.Errorf("failed to parse endpoints: %w", err)
	}

	// Calculate request interval
	interval := time.Second / time.Duration(r.cfg.TargetRPS)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Start worker goroutines
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(ctx, r.cfg.Duration)
	defer cancel()

	// Start concurrent workers
	for i := 0; i < r.cfg.Concurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.worker(ctx, endpoints, ticker.C)
		}()
	}

	wg.Wait()
	return nil
}

// runSpikeTest runs a spike test with sudden bursts.
func (r *Runner) runSpikeTest(ctx context.Context) error {
	endpoints, err := r.parseEndpoints()
	if err != nil {
		return fmt.Errorf("failed to parse endpoints: %w", err)
	}

	// Run baseline load
	baselineInterval := time.Second / time.Duration(r.cfg.TargetRPS)
	baselineTicker := time.NewTicker(baselineInterval)
	defer baselineTicker.Stop()

	ctx, cancel := context.WithTimeout(ctx, r.cfg.Duration)
	defer cancel()

	var wg sync.WaitGroup

	// Start baseline workers
	for i := 0; i < r.cfg.Concurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.worker(ctx, endpoints, baselineTicker.C)
		}()
	}

	// Start spike goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.runSpikes(ctx, endpoints)
	}()

	wg.Wait()
	return nil
}

// runSpikes runs spike bursts during the test.
func (r *Runner) runSpikes(ctx context.Context, endpoints []Endpoint) {
	if len(endpoints) == 0 {
		return
	}

	// Run spikes periodically
	ticker := time.NewTicker(r.cfg.SpikeDuration * 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Burst of requests at spike RPS
			spikeInterval := time.Second / time.Duration(r.cfg.SpikeRPS)
			spikeTicker := time.NewTicker(spikeInterval)
			spikeCtx, spikeCancel := context.WithTimeout(ctx, r.cfg.SpikeDuration)

			var spikeWg sync.WaitGroup
			for i := 0; i < r.cfg.Concurrent*5; i++ {
				spikeWg.Add(1)
				go func(idx int) {
					defer spikeWg.Done()
					for {
						select {
						case <-spikeCtx.Done():
							return
						case <-spikeTicker.C:
							ep := endpoints[idx%len(endpoints)]
							r.makeRequest(spikeCtx, ep)
						}
					}
				}(i)
			}

			spikeWg.Wait()
			spikeCancel()
			spikeTicker.Stop()
		}
	}
}

// runEnduranceTest runs a long-running test to detect memory leaks.
func (r *Runner) runEnduranceTest(ctx context.Context) error {
	endpoints, err := r.parseEndpoints()
	if err != nil {
		return fmt.Errorf("failed to parse endpoints: %w", err)
	}

	interval := time.Second / time.Duration(r.cfg.TargetRPS)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(ctx, r.cfg.Duration)
	defer cancel()

	for i := 0; i < r.cfg.Concurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.worker(ctx, endpoints, ticker.C)
		}()
	}

	wg.Wait()
	return nil
}

// worker runs requests in a loop until context is canceled.
func (r *Runner) worker(ctx context.Context, endpoints []Endpoint, ticker <-chan time.Time) {
	index := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker:
			if len(endpoints) == 0 {
				continue
			}
			ep := endpoints[index%len(endpoints)]
			index++
			r.makeRequest(ctx, ep)
		}
	}
}

// makeRequest makes a single HTTP request and records metrics.
func (r *Runner) makeRequest(ctx context.Context, ep Endpoint) {
	start := time.Now()

	// Construct URL - handle both ":8080" and "localhost:8080" formats
	addr := r.cfg.ServerAddr
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	url := "http://" + addr + ep.Path

	var body io.Reader
	if ep.Body != "" {
		body = bytes.NewBufferString(ep.Body)
	}

	req, err := http.NewRequestWithContext(ctx, ep.Method, url, body)
	if err != nil {
		r.metrics.RecordError(0)
		return
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := r.client.Do(req)
	latency := time.Since(start)

	if err != nil {
		r.metrics.RecordError(0)
		return
	}
	defer resp.Body.Close()

	// Read response body (discard it)
	_, _ = io.Copy(io.Discard, resp.Body)

	r.metrics.RecordRequest(latency, resp.StatusCode)
}

// parseEndpoints parses all endpoint strings.
func (r *Runner) parseEndpoints() ([]Endpoint, error) {
	endpoints := make([]Endpoint, 0, len(r.cfg.Endpoints))
	for _, s := range r.cfg.Endpoints {
		ep, err := ParseEndpoint(s)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, ep)
	}
	return endpoints, nil
}

// GenerateTestData generates test data for POST/PUT requests.
func GenerateTestData(endpoint string) (string, error) {
	data := map[string]any{
		"name":  "test",
		"value": "test",
		"id":    1,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}
