package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/kolosys/helix-stress-test/internal/config"
	"github.com/kolosys/helix-stress-test/internal/metrics"
	"github.com/kolosys/helix-stress-test/internal/report"
	"github.com/kolosys/helix-stress-test/internal/runner"
	"github.com/kolosys/helix-stress-test/server"
)

func main() {
	// Parse configuration
	cfg, err := config.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing configuration: %v\n", err)
		os.Exit(1)
	}

	// Create metrics collector
	m := metrics.New()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Get log file path before starting server
	logFilePath := server.GetLogFilePath(string(cfg.TestType))

	// Start server in background
	serverCtx, serverCancel := context.WithCancel(ctx)
	var serverWg sync.WaitGroup
	var logCleanup func() error
	serverWg.Add(1)
	go func() {
		defer serverWg.Done()
		_, cleanup, err := server.StartServer(serverCtx, cfg.ServerAddr, cfg.DatasetSize, string(cfg.TestType))
		if cleanup != nil {
			logCleanup = cleanup
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		}
	}()

	// Wait a moment for server to start
	time.Sleep(500 * time.Millisecond)

	// Create runner
	r := runner.New(cfg, m)

	// Start progress reporting
	progressDone := make(chan struct{})
	var progressWg sync.WaitGroup
	progressWg.Add(1)
	go func() {
		defer progressWg.Done()
		report.PrintProgress(m, 1*time.Second, progressDone)
	}()

	// Run stress test
	startTime := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("[%s] Starting stress test (type: %s, duration: %s, RPS: %d, concurrent: %d, dataset: %d items)...\n",
		startTime, cfg.TestType, cfg.Duration, cfg.TargetRPS, cfg.Concurrent, cfg.DatasetSize)
	if logFilePath != "" {
		fmt.Printf("Server logs: %s\n", logFilePath)
	}
	fmt.Println()

	var testWg sync.WaitGroup
	testWg.Add(1)
	testDone := make(chan struct{})
	go func() {
		defer testWg.Done()
		defer close(testDone)
		if err := r.Run(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Test error: %v\n", err)
		}
	}()

	// Wait for test completion or signal
	select {
	case <-testDone:
		// Test completed normally (runner's internal timeout expired)
	case <-sigCh:
		// Received interrupt signal
		fmt.Println("\nReceived interrupt signal, shutting down...")
		cancel()
		// Wait for test to finish after cancellation
		<-testDone
	}

	// Stop progress reporting
	close(progressDone)
	progressWg.Wait()
	fmt.Println()

	// Wait for test to finish
	testWg.Wait()

	// Generate report
	reportTime := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("[%s] Generating report...\n", reportTime)
	gen := report.New(cfg, m)
	if err := gen.Generate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating report: %v\n", err)
		os.Exit(1)
	}

	// Shutdown server
	serverCancel()
	serverWg.Wait()

	// Close log file
	if logCleanup != nil {
		if err := logCleanup(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close log file: %v\n", err)
		}
	}
}
