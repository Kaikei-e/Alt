package test_utils

import (
	"alt/domain"
	"context"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLogger creates a logger for testing that outputs to testing.T
func TestLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// SilentTestLogger creates a logger that discards output for performance tests
func SilentTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError, // Only log errors
	}))
}

// AssertErrorContains verifies that an error contains a specific message
func AssertErrorContains(t *testing.T, err error, expectedMessage string) {
	t.Helper()
	require.Error(t, err, "Expected an error but got nil")
	assert.Contains(t, err.Error(), expectedMessage, "Error message doesn't contain expected text")
}

// AssertErrorType verifies that an error is of a specific type
func AssertErrorType(t *testing.T, err error, expectedType interface{}) {
	t.Helper()
	require.Error(t, err, "Expected an error but got nil")

	expectedTypeName := reflect.TypeOf(expectedType).String()
	actualTypeName := reflect.TypeOf(err).String()

	assert.Equal(t, expectedTypeName, actualTypeName,
		"Error type mismatch: expected %s, got %s", expectedTypeName, actualTypeName)
}

// AssertWithinDuration verifies that an operation completes within expected time
func AssertWithinDuration(t *testing.T, expectedDuration time.Duration, tolerance time.Duration, operation func()) {
	t.Helper()
	start := time.Now()
	operation()
	actual := time.Since(start)

	minDuration := expectedDuration - tolerance
	maxDuration := expectedDuration + tolerance

	assert.True(t, actual >= minDuration && actual <= maxDuration,
		"Operation took %v, expected %v ± %v", actual, expectedDuration, tolerance)
}

// WaitForCondition waits for a condition to become true within a timeout
func WaitForCondition(t *testing.T, timeout time.Duration, condition func() bool, message string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Condition not met within %v: %s", timeout, message)
		case <-ticker.C:
			if condition() {
				return
			}
		}
	}
}

// CreateTestContext creates a context with test-specific values
func CreateTestContext(t *testing.T) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "test_name", t.Name())
	ctx = context.WithValue(ctx, "request_id", fmt.Sprintf("test-%d", time.Now().UnixNano()))
	return ctx
}

// CreateTestContextWithTimeout creates a context with timeout for long-running tests
func CreateTestContextWithTimeout(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx := CreateTestContext(t)
	return context.WithTimeout(ctx, timeout)
}

// AssertSliceEqual compares two slices with custom comparison function
func AssertSliceEqual[T any](t *testing.T, expected, actual []T, compareFn func(T, T) bool, message string) {
	t.Helper()

	if len(expected) != len(actual) {
		t.Fatalf("%s: length mismatch - expected %d, got %d", message, len(expected), len(actual))
	}

	for i := range expected {
		if !compareFn(expected[i], actual[i]) {
			t.Errorf("%s: mismatch at index %d - expected %+v, got %+v",
				message, i, expected[i], actual[i])
		}
	}
}

// GenerateTestData creates test data for various scenarios
type TestDataGenerator struct {
	counter int
}

func NewTestDataGenerator() *TestDataGenerator {
	return &TestDataGenerator{counter: 0}
}

func (g *TestDataGenerator) NextFeed() domain.RSSFeed {
	g.counter++
	return domain.RSSFeed{
		Title:         fmt.Sprintf("Test Feed %d", g.counter),
		Description:   fmt.Sprintf("Description for test feed %d", g.counter),
		Link:          fmt.Sprintf("http://example.com/feed%d", g.counter),
		Updated:       time.Now().Add(-time.Duration(g.counter) * time.Hour).Format(time.RFC3339),
		UpdatedParsed: time.Now().Add(-time.Duration(g.counter) * time.Hour),
	}
}

func (g *TestDataGenerator) NextFeeds(count int) []domain.RSSFeed {
	feeds := make([]domain.RSSFeed, count)
	for i := 0; i < count; i++ {
		feeds[i] = g.NextFeed()
	}
	return feeds
}

func (g *TestDataGenerator) NextURL() string {
	g.counter++
	return fmt.Sprintf("http://example%d.com/feed.xml", g.counter)
}

func (g *TestDataGenerator) NextInvalidURL() string {
	g.counter++
	invalidURLs := []string{
		"not-a-url",
		"ftp://example.com/feed.xml",
		"http://192.168.1.1/feed.xml",
		"http://localhost/feed.xml",
		"",
	}
	return invalidURLs[g.counter%len(invalidURLs)]
}

// Table-driven test helpers
type TestCase[I, O any] struct {
	Name        string
	Input       I
	Expected    O
	ShouldError bool
	ErrorMsg    string
	Setup       func(*testing.T) error
	Cleanup     func(*testing.T) error
}

func RunTestCases[I, O any](t *testing.T, testCases []TestCase[I, O], testFunc func(I) (O, error)) {
	t.Helper()

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			// Setup
			if tc.Setup != nil {
				err := tc.Setup(t)
				require.NoError(t, err, "Test setup failed")
			}

			// Cleanup
			if tc.Cleanup != nil {
				defer func() {
					err := tc.Cleanup(t)
					assert.NoError(t, err, "Test cleanup failed")
				}()
			}

			// Execute test
			result, err := testFunc(tc.Input)

			// Verify results
			if tc.ShouldError {
				require.Error(t, err, "Expected error but got nil")
				if tc.ErrorMsg != "" {
					assert.Contains(t, err.Error(), tc.ErrorMsg)
				}
			} else {
				require.NoError(t, err, "Unexpected error: %v", err)
				assert.Equal(t, tc.Expected, result)
			}
		})
	}
}

// Concurrency test helpers
func RunConcurrentTest(t *testing.T, goroutineCount int, operationsPerGoroutine int, operation func(int, int) error) {
	t.Helper()

	errors := make(chan error, goroutineCount*operationsPerGoroutine)
	done := make(chan bool, goroutineCount)

	// Start goroutines
	for i := 0; i < goroutineCount; i++ {
		go func(goroutineID int) {
			defer func() { done <- true }()

			for j := 0; j < operationsPerGoroutine; j++ {
				if err := operation(goroutineID, j); err != nil {
					errors <- err
				}
			}
		}(i)
	}

	// Wait for completion
	for i := 0; i < goroutineCount; i++ {
		<-done
	}
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Errorf("Concurrent operation failed: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Fatalf("Found %d errors in concurrent test", errorCount)
	}
}

// Performance measurement helpers
type PerformanceMetrics struct {
	Duration   time.Duration
	Operations int
	Throughput float64
	MemoryUsed int64
}

func MeasurePerformance(operations int, operation func() error) (PerformanceMetrics, error) {
	start := time.Now()

	for i := 0; i < operations; i++ {
		if err := operation(); err != nil {
			return PerformanceMetrics{}, fmt.Errorf("operation %d failed: %w", i, err)
		}
	}

	duration := time.Since(start)
	throughput := float64(operations) / duration.Seconds()

	return PerformanceMetrics{
		Duration:   duration,
		Operations: operations,
		Throughput: throughput,
	}, nil
}

// Mock data generators for specific domains
func GenerateMockRSSFeedXML(itemCount int) string {
	var builder strings.Builder

	builder.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
    <channel>
        <title>Mock RSS Feed</title>
        <description>A mock RSS feed for testing</description>
        <link>http://mock.example.com</link>`)

	for i := 0; i < itemCount; i++ {
		builder.WriteString(fmt.Sprintf(`
        <item>
            <title>Mock Article %d</title>
            <description><![CDATA[This is mock article number %d with some <strong>HTML content</strong>.]]></description>
            <link>http://mock.example.com/article%d</link>
            <pubDate>%s</pubDate>
        </item>`,
			i+1, i+1, i+1,
			time.Now().Add(-time.Duration(i)*time.Hour).Format(time.RFC1123Z)))
	}

	builder.WriteString(`
    </channel>
</rss>`)

	return builder.String()
}

// Environment helpers for tests
func SetTestEnv(t *testing.T, key, value string) {
	t.Helper()
	original := os.Getenv(key)
	os.Setenv(key, value)

	t.Cleanup(func() {
		if original == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, original)
		}
	})
}

func ClearTestEnv(t *testing.T, key string) {
	t.Helper()
	original := os.Getenv(key)
	os.Unsetenv(key)

	t.Cleanup(func() {
		if original != "" {
			os.Setenv(key, original)
		}
	})
}

// Timeout helpers for different test types
const (
	FastTestTimeout   = 1 * time.Second
	NormalTestTimeout = 5 * time.Second
	SlowTestTimeout   = 30 * time.Second
)

func GetTestTimeout(testType string) time.Duration {
	switch testType {
	case "unit":
		return FastTestTimeout
	case "integration":
		return NormalTestTimeout
	case "performance":
		return SlowTestTimeout
	default:
		return NormalTestTimeout
	}
}

// Test result verification helpers
func AssertFeedEqual(t *testing.T, expected, actual domain.RSSFeed) {
	t.Helper()
	assert.Equal(t, expected.Title, actual.Title, "Feed title mismatch")
	assert.Equal(t, expected.Description, actual.Description, "Feed description mismatch")
	assert.Equal(t, expected.Link, actual.Link, "Feed link mismatch")
	assert.True(t, expected.UpdatedParsed.Equal(actual.UpdatedParsed),
		"Feed updated time mismatch: expected %v, got %v", expected.UpdatedParsed, actual.UpdatedParsed)
}
