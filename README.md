# Helix Stress Test

Comprehensive stress testing automation suite for the [helix](https://github.com/kolosys/helix) web framework. This tool exercises all major helix features under various load conditions to validate performance, concurrency, and stability.

## Features

- **Zero External Dependencies**: Uses only Go stdlib and the helix library
- **Comprehensive Test Coverage**: Tests all major helix features including routes, middleware, request binding, and error handling
- **Multiple Test Types**: Supports load, spike, and endurance testing
- **Detailed Metrics**: Tracks latency percentiles, throughput, error rates, and memory usage
- **Flexible Configuration**: Command-line flags and environment variable support
- **Multiple Report Formats**: Text and JSON output formats

## Installation

```bash
cd helix-stress-test
go mod tidy
go build
```

## Quick Start

```bash
# Run a basic load test (60 seconds, 100 RPS, 10 concurrent connections)
# Report is automatically saved to results/load-test.txt
go run . --type=load --duration=60s --rps=100 --concurrent=10

# Run an endurance test (10 minutes)
# Report is automatically saved to results/endurance-test.txt
go run . --type=endurance --duration=10m --rps=50

# Run a spike test
# Report is automatically saved to results/spike-test.txt
go run . --type=spike --duration=60s --rps=100 --spike-rps=1000 --spike-duration=5s

# Output to stdout instead of file
go run . --type=load --duration=10s --output=""
```

## Usage

### Command-Line Options

```
  -server-addr string
        Server address to test (default ":8080")
  -type string
        Test type: load, spike, or endurance (default "load")
  -duration duration
        Test duration (default 60s)
  -rps int
        Target requests per second (default 100)
  -concurrent int
        Number of concurrent connections (default 10)
  -spike-duration duration
        Spike test duration (default 5s)
  -spike-rps int
        Spike test RPS (default 1000)
  -timeout duration
        Request timeout (default 30s)
  -format string
        Report format: text, json (default "text")
  -output string
        Output file for report (default: results/{type}-test.{format}, empty for stdout)
  -endpoints string
        Comma-separated list of endpoints (e.g., GET:/,POST:/items)
  -dataset-size int
        Number of items to pre-populate (0 for empty store) (default 10000)
```

### Environment Variables

All command-line options can also be set via environment variables:

- `SERVER_ADDR` - Server address
- `TEST_TYPE` - Test type (load/spike/endurance)
- `DURATION` - Test duration (e.g., "60s", "10m")
- `TARGET_RPS` - Target requests per second
- `CONCURRENT` - Number of concurrent connections
- `SPIKE_DURATION` - Spike test duration
- `SPIKE_RPS` - Spike test RPS
- `TIMEOUT` - Request timeout
- `REPORT_FORMAT` - Report format (text/json)
- `REPORT_FILE` - Output file path (default: results/{type}-test.{format})
- `DATASET_SIZE` - Number of items to pre-populate (default: 10000)
- `ENDPOINTS` - Comma-separated endpoint list

## Test Types

### Load Test

Sustained high request rate for a fixed duration. Useful for testing steady-state performance.

```bash
go run . --type=load --duration=60s --rps=1000 --concurrent=50
```

### Spike Test

Baseline load with periodic spikes of high traffic. Useful for testing how the system handles sudden load increases.

```bash
go run . --type=spike --duration=120s --rps=100 --spike-rps=5000 --spike-duration=5s
```

### Endurance Test

Long-running test at moderate load. Useful for detecting memory leaks and stability issues.

```bash
go run . --type=endurance --duration=30m --rps=50 --concurrent=10
```

## Test Endpoints

The stress test server exposes various endpoints to test different helix features:

### Basic Routes

- `GET /` - Simple GET handler
- `GET /ping` - Ping endpoint
- `GET /health` - Health check

### Path Parameters

- `GET /users/{id}` - Single path parameter
- `GET /users/{id}/posts/{postID}` - Multiple path parameters
- `GET /categories/{category}/items/{id}` - Path parameters with binding

### Query Parameters

- `GET /search?q=test&limit=10` - Query string parsing
- `GET /api/search?search=test&sort=name&order=asc` - Query binding

### Request Binding

- `POST /items` - JSON body binding
- `PUT /items/{id}` - JSON body + path binding
- `GET /items` - Query parameter binding
- `GET /items/{id}` - Path parameter binding

### Middleware

- `GET /middleware/test` - Middleware chain test

### Error Handling

- `GET /error/400` - Bad request error
- `GET /error/404` - Not found error
- `GET /error/500` - Internal server error

### Resource Routes

- `GET /products` - List resources
- `POST /products` - Create resource
- `GET /products/{id}` - Get resource
- `PUT /products/{id}` - Update resource
- `DELETE /products/{id}` - Delete resource

## Custom Endpoints

You can specify custom endpoints to test:

```bash
go run . --endpoints="GET:/,GET:/users/123,POST:/items,PUT:/items/1"
```

Endpoint format: `METHOD:PATH` (e.g., `GET:/users/123`, `POST:/items`)

## Metrics

The stress test collects comprehensive metrics:

### Request Statistics

- Total requests
- Success requests (2xx, 3xx)
- Error requests (4xx, 5xx)
- Error rate percentage

### Throughput

- Current RPS (requests per second)
- Average RPS

### Latency

- Min, Mean, Max
- Percentiles: P50, P95, P99, P99.9

### Error Breakdown

- Error count by HTTP status code

### Memory Statistics

- Allocated memory
- Total allocations
- System memory
- GC cycles and rate

## Report Formats

### Text Report (Default)

Human-readable text report with all metrics:

```bash
go run . --format=text
```

### JSON Report

Machine-readable JSON output (automatically saved to results/{type}-test.json):

```bash
# Default location: results/load-test.json
go run . --type=load --format=json

# Custom location
go run . --format=json --output=custom-report.json
```

## Examples

### Example 1: Basic Load Test

```bash
go run . \
  --type=load \
  --duration=60s \
  --rps=500 \
  --concurrent=25
```

### Example 2: High-Throughput Test

```bash
go run . \
  --type=load \
  --duration=30s \
  --rps=5000 \
  --concurrent=100 \
  --timeout=10s
```

### Example 3: Endurance Test with JSON Output

```bash
go run . \
  --type=endurance \
  --duration=1h \
  --rps=100 \
  --concurrent=20 \
  --format=json \
  --output=endurance-report.json
```

### Example 4: Spike Test

```bash
go run . \
  --type=spike \
  --duration=5m \
  --rps=200 \
  --spike-rps=2000 \
  --spike-duration=10s \
  --concurrent=50
```

### Example 5: Custom Endpoints

```bash
go run . \
  --type=load \
  --duration=30s \
  --rps=1000 \
  --endpoints="GET:/ping,GET:/users/123,POST:/items,GET:/items/1"
```

## Architecture

The stress test suite consists of:

1. **Test Server** (`server/main.go`) - Helix server with comprehensive endpoints
2. **Stress Test Runner** (`runner/runner.go`) - HTTP client that generates load
3. **Metrics Collector** (`metrics/metrics.go`) - Collects and aggregates metrics
4. **Report Generator** (`report/report.go`) - Generates test reports
5. **Configuration** (`config/config.go`) - Configuration management
6. **Main Entry Point** (`main.go`) - Orchestrates test execution

## Test Scenarios

The stress test covers:

1. **Basic GET requests** - Simple route handling
2. **POST with JSON binding** - Request body parsing
3. **Path parameters** - Dynamic route matching
4. **Query parameters** - Query string parsing
5. **Middleware chains** - Multiple middleware layers
6. **Error responses** - Error handling paths
7. **Concurrent requests** - Thread safety
8. **Long-running test** - Memory leak detection

## Performance Considerations

- Uses connection pooling for efficient HTTP requests
- Thread-safe metrics collection
- Zero-allocation hot paths where possible
- Context-aware cancellation for graceful shutdown

## Contributing

This stress test suite follows Kolosys coding standards:

- Zero external dependencies
- Context-aware operations
- Thread-safe implementations
- Comprehensive error handling
- Production-ready code quality

## License

MIT License - see LICENSE file for details.
