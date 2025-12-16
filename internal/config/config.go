package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// TestType represents the type of stress test to run.
type TestType string

const (
	TestTypeLoad      TestType = "load"
	TestTypeSpike     TestType = "spike"
	TestTypeEndurance TestType = "endurance"
)

// Config holds all configuration for the stress test.
type Config struct {
	// Server configuration
	ServerAddr string

	// Test configuration
	TestType      TestType
	Duration      time.Duration
	TargetRPS     int
	Concurrent    int
	SpikeDuration time.Duration
	SpikeRPS      int

	// Request configuration
	Timeout time.Duration

	// Report configuration
	ReportFormat string
	ReportFile   string

	// Endpoints to test
	Endpoints []string

	// Dataset configuration
	DatasetSize int // Number of items to pre-populate (0 for empty store)
}

// Default returns a Config with default values.
func Default() *Config {
	return &Config{
		ServerAddr:    ":8080",
		TestType:      TestTypeLoad,
		Duration:      60 * time.Second,
		TargetRPS:     100,
		Concurrent:    10,
		SpikeDuration: 5 * time.Second,
		SpikeRPS:      1000,
		Timeout:       30 * time.Second,
		ReportFormat:  "text",
		ReportFile:    "",
		Endpoints: []string{
			"GET:/",
			"GET:/ping",
			"GET:/users/123",
			"GET:/search?q=test&limit=10",
			"GET:/items/1",
			"POST:/items",
			"PUT:/items/1",
			"DELETE:/items/1",
		},
		DatasetSize: 10000, // Pre-populate with 10,000 items by default
	}
}

// Parse parses command-line flags and environment variables into Config.
func Parse() (*Config, error) {
	cfg := Default()

	// Command-line flags
	flag.StringVar(&cfg.ServerAddr, "server-addr", getEnv("SERVER_ADDR", cfg.ServerAddr), "Server address to test")
	flag.StringVar((*string)(&cfg.TestType), "type", getEnv("TEST_TYPE", string(cfg.TestType)), "Test type: load, spike, or endurance")
	flag.DurationVar(&cfg.Duration, "duration", parseDurationEnv("DURATION", cfg.Duration), "Test duration")
	flag.IntVar(&cfg.TargetRPS, "rps", parseIntEnv("TARGET_RPS", cfg.TargetRPS), "Target requests per second")
	flag.IntVar(&cfg.Concurrent, "concurrent", parseIntEnv("CONCURRENT", cfg.Concurrent), "Number of concurrent connections")
	flag.DurationVar(&cfg.SpikeDuration, "spike-duration", parseDurationEnv("SPIKE_DURATION", cfg.SpikeDuration), "Spike test duration")
	flag.IntVar(&cfg.SpikeRPS, "spike-rps", parseIntEnv("SPIKE_RPS", cfg.SpikeRPS), "Spike test RPS")
	flag.DurationVar(&cfg.Timeout, "timeout", parseDurationEnv("TIMEOUT", cfg.Timeout), "Request timeout")
	flag.StringVar(&cfg.ReportFormat, "format", getEnv("REPORT_FORMAT", cfg.ReportFormat), "Report format: text, json")
	flag.StringVar(&cfg.ReportFile, "output", getEnv("REPORT_FILE", cfg.ReportFile), "Output file for report (default: results/{type}-test.{format}, empty for stdout)")
	flag.IntVar(&cfg.DatasetSize, "dataset-size", parseIntEnv("DATASET_SIZE", cfg.DatasetSize), "Number of items to pre-populate (0 for empty store)")

	var endpointsFlag string
	flag.StringVar(&endpointsFlag, "endpoints", getEnv("ENDPOINTS", ""), "Comma-separated list of endpoints (e.g., GET:/,POST:/items)")

	flag.Parse()

	// Parse endpoints
	if endpointsFlag != "" {
		cfg.Endpoints = parseEndpoints(endpointsFlag)
	} else if envEndpoints := getEnv("ENDPOINTS", ""); envEndpoints != "" {
		cfg.Endpoints = parseEndpoints(envEndpoints)
	}

	// Set default output file if not specified
	if cfg.ReportFile == "" {
		// Ensure results directory exists
		resultsDir := "results"
		if err := os.MkdirAll(resultsDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create results directory: %w", err)
		}

		// Generate filename based on test type and format
		ext := "txt"
		if cfg.ReportFormat == "json" {
			ext = "json"
		}
		cfg.ReportFile = filepath.Join(resultsDir, fmt.Sprintf("%s-test.%s", cfg.TestType, ext))
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.ServerAddr == "" {
		return fmt.Errorf("server address cannot be empty")
	}

	switch c.TestType {
	case TestTypeLoad, TestTypeSpike, TestTypeEndurance:
		// Valid
	default:
		return fmt.Errorf("invalid test type: %s (must be load, spike, or endurance)", c.TestType)
	}

	if c.Duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}

	if c.TargetRPS <= 0 {
		return fmt.Errorf("target RPS must be positive")
	}

	if c.Concurrent <= 0 {
		return fmt.Errorf("concurrent connections must be positive")
	}

	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	switch c.ReportFormat {
	case "text", "json":
		// Valid
	default:
		return fmt.Errorf("invalid report format: %s (must be text or json)", c.ReportFormat)
	}

	if len(c.Endpoints) == 0 {
		return fmt.Errorf("at least one endpoint must be specified")
	}

	return nil
}

// getEnv gets an environment variable or returns the default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// parseIntEnv parses an integer environment variable or returns the default value.
func parseIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// parseDurationEnv parses a duration environment variable or returns the default value.
func parseDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// parseEndpoints parses a comma-separated list of endpoints.
func parseEndpoints(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	endpoints := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			endpoints = append(endpoints, part)
		}
	}
	return endpoints
}
