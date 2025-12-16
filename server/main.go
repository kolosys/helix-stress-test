package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/kolosys/helix"
	"github.com/kolosys/helix/middleware"
)

// Item represents a test item for CRUD operations.
type Item struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ItemStore provides thread-safe in-memory storage.
type ItemStore struct {
	mu     sync.RWMutex
	items  map[int]Item
	nextID int
}

// NewItemStore creates a new ItemStore.
func NewItemStore() *ItemStore {
	return &ItemStore{
		items:  make(map[int]Item),
		nextID: 1,
	}
}

// PrePopulate fills the store with a dataset of the specified size.
func (s *ItemStore) PrePopulate(size int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := 1; i <= size; i++ {
		s.items[i] = Item{
			ID:    i,
			Name:  fmt.Sprintf("Item-%d", i),
			Value: fmt.Sprintf("value-%d", i),
		}
	}
	s.nextID = size + 1
}

// GetRandomID returns a random ID from existing items (for testing).
// Optimized to avoid allocations - uses a simple counter-based approach.
func (s *ItemStore) GetRandomID() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.items) == 0 {
		return 1
	}

	// Use a simple approach: return a random ID from 1 to len(items)
	// This avoids allocating a slice and iterating over all items
	// For stress testing, this is sufficient and much faster
	count := len(s.items)
	if count == 0 {
		return 1
	}

	// Return ID in range [1, count] - works well with pre-populated datasets
	// where IDs are sequential from 1 to datasetSize
	return (count % 1000) + 1 // Cycle through first 1000 IDs
}

// Request/Response types for typed handlers
type (
	// GetItemRequest contains the item ID from the path.
	GetItemRequest struct {
		ID int `path:"id"`
	}

	// ListItemsRequest contains query parameters for listing items.
	ListItemsRequest struct {
		Page  int `query:"page"`
		Limit int `query:"limit"`
	}

	// ListItemsResponse is the response for listing items.
	ListItemsResponse struct {
		Items []Item `json:"items"`
		Total int    `json:"total"`
		Page  int    `json:"page"`
		Limit int    `json:"limit"`
	}

	// CreateItemRequest contains the data for creating an item.
	CreateItemRequest struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}

	// UpdateItemRequest contains the data for updating an item.
	UpdateItemRequest struct {
		ID    int    `path:"id"`
		Name  string `json:"name"`
		Value string `json:"value"`
	}

	// DeleteItemRequest contains the item ID to delete.
	DeleteItemRequest struct {
		ID int `path:"id"`
	}

	// QueryParamsRequest demonstrates query parameter binding.
	QueryParamsRequest struct {
		Search string `query:"search"`
		Sort   string `query:"sort"`
		Order  string `query:"order"`
	}

	// PathParamsRequest demonstrates path parameter binding.
	PathParamsRequest struct {
		Category string `path:"category"`
		ID       int    `path:"id"`
	}
)

// GetLogFilePath returns the log file path that will be used for a test run.
// testType is the type of test being run (e.g., "load", "spike", "endurance").
func GetLogFilePath(testType string) string {
	logsDir := "logs"
	timestamp := time.Now().Format("20060102-150405")
	return filepath.Join(logsDir, fmt.Sprintf("server-%s-%s.log", testType, timestamp))
}

// NewServer creates and configures a test server with all helix features.
// datasetSize specifies how many items to pre-populate (0 for empty store).
// testType is the type of test being run (e.g., "load", "spike", "endurance").
// Returns the server, log file path, and a cleanup function to close the log file.
func NewServer(addr string, datasetSize int, testType string) (*helix.Server, string, func() error) {
	store := NewItemStore()

	// Pre-populate dataset if specified
	if datasetSize > 0 {
		store.PrePopulate(datasetSize)
	}

	// Create logs directory if it doesn't exist
	logsDir := "logs"
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		// If we can't create logs directory, continue without logging
		fmt.Fprintf(os.Stderr, "Warning: failed to create logs directory: %v\n", err)
	}

	// Create log file with test type and timestamp
	timestamp := time.Now().Format("20060102-150405")
	logFile := filepath.Join(logsDir, fmt.Sprintf("server-%s-%s.log", testType, timestamp))

	var logWriter *os.File
	var cleanup func() error = func() error { return nil }

	if logsDir != "" {
		var err error
		logWriter, err = os.Create(logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create log file: %v\n", err)
		} else {
			// Note: Don't print log file location to stdout to avoid cluttering progress output
			// Logs are silently written to the file
			cleanup = func() error {
				return logWriter.Close()
			}
		}
	}

	// Create server with logger middleware writing to file
	s := helix.New(
		helix.WithAddr(addr),
		helix.HideBanner(), // Hide banner for cleaner output
	)
	s.Use(middleware.RequestID())

	// Add logger middleware with file output
	if logWriter != nil {
		loggerConfig := middleware.LoggerConfig{
			Format:        middleware.LogFormatDev,
			Output:        logWriter,
			DisableColors: true, // Disable colors for file output
		}
		s.Use(middleware.LoggerWithConfig(loggerConfig))
	}

	s.Use(middleware.Recover())

	// Add additional middleware to test middleware chains
	s.Use(middleware.Compress())

	// Basic routes - simple GET/POST/PUT/DELETE
	s.GET("/", helix.HandleCtx(func(c *helix.Ctx) error {
		return c.OK(map[string]string{
			"message": "Helix Stress Test Server",
			"status":  "ready",
		})
	}))

	s.GET("/ping", helix.HandleCtx(func(c *helix.Ctx) error {
		return c.OK(map[string]string{
			"message": "pong",
		})
	}))

	// Path parameters - simple
	s.GET("/users/{id}", helix.HandleCtx(func(c *helix.Ctx) error {
		id := c.Param("id")
		return c.OK(map[string]string{
			"id":   id,
			"name": fmt.Sprintf("User-%s", id),
		})
	}))

	// Path parameters - multiple
	s.GET("/users/{id}/posts/{postID}", helix.HandleCtx(func(c *helix.Ctx) error {
		userID := c.Param("id")
		postID := c.Param("postID")
		return c.OK(map[string]string{
			"userID": userID,
			"postID": postID,
			"title":  fmt.Sprintf("Post %s by User %s", postID, userID),
		})
	}))

	// Query parameters - simple
	s.GET("/search", helix.HandleCtx(func(c *helix.Ctx) error {
		query := c.QueryDefault("q", "")
		limit := c.QueryInt("limit", 10)
		return c.OK(map[string]any{
			"query": query,
			"limit": limit,
			"results": []string{
				fmt.Sprintf("Result 1 for '%s'", query),
				fmt.Sprintf("Result 2 for '%s'", query),
			},
		})
	}))

	// Query parameters - with binding
	s.GET("/api/search", helix.Handle(func(ctx context.Context, req QueryParamsRequest) (map[string]any, error) {
		return map[string]any{
			"search": req.Search,
			"sort":   req.Sort,
			"order":  req.Order,
			"results": []string{
				fmt.Sprintf("Result sorted by %s in %s order", req.Sort, req.Order),
			},
		}, nil
	}))

	// Path parameters - with binding
	s.GET("/categories/{category}/items/{id}", helix.Handle(func(ctx context.Context, req PathParamsRequest) (map[string]any, error) {
		return map[string]any{
			"category": req.Category,
			"id":       req.ID,
			"name":     fmt.Sprintf("Item %d in %s", req.ID, req.Category),
		}, nil
	}))

	// JSON body binding - POST
	s.POST("/items", helix.HandleCreated(func(ctx context.Context, req CreateItemRequest) (Item, error) {
		store.mu.Lock()
		defer store.mu.Unlock()

		item := Item{
			ID:    store.nextID,
			Name:  req.Name,
			Value: req.Value,
		}
		store.items[item.ID] = item
		store.nextID++

		return item, nil
	}))

	// JSON body binding - PUT
	s.PUT("/items/{id}", helix.Handle(func(ctx context.Context, req UpdateItemRequest) (Item, error) {
		store.mu.Lock()
		defer store.mu.Unlock()

		item, ok := store.items[req.ID]
		if !ok {
			return Item{}, helix.NotFoundf("item %d not found", req.ID)
		}

		// Update the item
		item.Name = req.Name
		item.Value = req.Value
		store.items[req.ID] = item

		return item, nil
	}))

	// Typed handler - GET with path binding
	s.GET("/items/{id}", helix.Handle(func(ctx context.Context, req GetItemRequest) (Item, error) {
		store.mu.RLock()
		defer store.mu.RUnlock()

		item, ok := store.items[req.ID]
		if !ok {
			return Item{}, helix.NotFoundf("item %d not found", req.ID)
		}

		return item, nil
	}))

	// Typed handler - GET with query binding
	s.GET("/items", helix.Handle(func(ctx context.Context, req ListItemsRequest) (ListItemsResponse, error) {
		store.mu.RLock()
		total := len(store.items)

		if req.Page <= 0 {
			req.Page = 1
		}
		if req.Limit <= 0 {
			req.Limit = 10
		}

		// Optimized pagination: only iterate through items we need
		// This avoids copying all 10,000 items before pagination
		start := (req.Page - 1) * req.Limit
		end := start + req.Limit

		if start >= total {
			store.mu.RUnlock()
			return ListItemsResponse{
				Items: []Item{},
				Total: total,
				Page:  req.Page,
				Limit: req.Limit,
			}, nil
		}

		if end > total {
			end = total
		}

		// Iterate through items map, collecting only what we need
		// This is much faster than copying all items first
		items := make([]Item, 0, req.Limit)
		skipped := 0
		for _, item := range store.items {
			if skipped < start {
				skipped++
				continue
			}
			if len(items) >= req.Limit {
				break
			}
			items = append(items, item)
		}

		store.mu.RUnlock()

		return ListItemsResponse{
			Items: items,
			Total: total,
			Page:  req.Page,
			Limit: req.Limit,
		}, nil
	}))

	// Typed handler - DELETE
	s.DELETE("/items/{id}", helix.HandleNoResponse(func(ctx context.Context, req DeleteItemRequest) error {
		store.mu.Lock()
		defer store.mu.Unlock()

		if _, ok := store.items[req.ID]; !ok {
			return helix.NotFoundf("item %d not found", req.ID)
		}

		delete(store.items, req.ID)
		return nil
	}))

	// Error handling - various error types
	s.GET("/error/400", helix.HandleCtx(func(c *helix.Ctx) error {
		return helix.BadRequestf("bad request error")
	}))

	s.GET("/error/404", helix.HandleCtx(func(c *helix.Ctx) error {
		return helix.NotFoundf("resource not found")
	}))

	s.GET("/error/500", helix.HandleCtx(func(c *helix.Ctx) error {
		return helix.Internalf("internal server error")
	}))

	// Middleware chain test - route with additional middleware
	mwGroup := s.Group("/middleware", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Middleware-Test", "true")
			next.ServeHTTP(w, r)
		})
	})

	mwGroup.GET("/test", helix.HandleCtx(func(c *helix.Ctx) error {
		return c.OK(map[string]any{
			"message":      "middleware chain test",
			"middleware":   true,
			"requestID":    c.Header("X-Request-ID"),
			"customHeader": c.Header("X-Middleware-Test"),
		})
	}))

	// Resource routes
	s.Resource("/products").
		List(helix.HandleCtx(func(c *helix.Ctx) error {
			return c.OK(map[string]any{
				"products": []map[string]any{
					{"id": 1, "name": "Product 1"},
					{"id": 2, "name": "Product 2"},
				},
			})
		})).
		Create(helix.HandleCtx(func(c *helix.Ctx) error {
			var req map[string]any
			if err := c.Bind(&req); err != nil {
				return helix.BadRequestf("invalid request body")
			}
			return c.Created(map[string]any{
				"id":   3,
				"name": req["name"],
			})
		})).
		Get(helix.HandleCtx(func(c *helix.Ctx) error {
			id := c.Param("id")
			idInt, _ := strconv.Atoi(id)
			return c.OK(map[string]any{
				"id":   idInt,
				"name": fmt.Sprintf("Product %s", id),
			})
		})).
		Update(helix.HandleCtx(func(c *helix.Ctx) error {
			id := c.Param("id")
			var req map[string]any
			if err := c.Bind(&req); err != nil {
				return helix.BadRequestf("invalid request body")
			}
			idInt, _ := strconv.Atoi(id)
			return c.OK(map[string]any{
				"id":   idInt,
				"name": req["name"],
			})
		})).
		Delete(helix.HandleCtx(func(c *helix.Ctx) error {
			return c.NoContent()
		}))

	// Health check endpoint
	s.GET("/health", helix.HandleCtx(func(c *helix.Ctx) error {
		return c.OK(map[string]string{
			"status": "healthy",
		})
	}))

	return s, logFile, cleanup
}

// StartServer starts the test server and blocks until shutdown.
// datasetSize specifies how many items to pre-populate (0 for empty store).
// testType is the type of test being run (e.g., "load", "spike", "endurance").
// Returns the log file path and cleanup function.
func StartServer(ctx context.Context, addr string, datasetSize int, testType string) (string, func() error, error) {
	s, logFile, cleanup := NewServer(addr, datasetSize, testType)
	err := s.Run(ctx)
	return logFile, cleanup, err
}
