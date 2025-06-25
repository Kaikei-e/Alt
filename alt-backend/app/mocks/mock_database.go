package mocks

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MockDatabase implements a fake database for testing
type MockDatabase struct {
	mu           sync.RWMutex
	tables       map[string][]map[string]interface{}
	sequences    map[string]int
	executed     []string
	shouldFail   bool
	failureError error
	latency      time.Duration
}

// NewMockDatabase creates a new mock database
func NewMockDatabase() *MockDatabase {
	return &MockDatabase{
		tables:    make(map[string][]map[string]interface{}),
		sequences: make(map[string]int),
		executed:  make([]string, 0),
		latency:   1 * time.Millisecond, // Default latency
	}
}

// SetLatency sets the simulated database latency
func (m *MockDatabase) SetLatency(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latency = latency
}

// SetShouldFail configures the mock to fail operations
func (m *MockDatabase) SetShouldFail(shouldFail bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail = shouldFail
	m.failureError = err
}

// GetExecutedQueries returns all executed queries for verification
func (m *MockDatabase) GetExecutedQueries() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	queries := make([]string, len(m.executed))
	copy(queries, m.executed)
	return queries
}

// ClearExecutedQueries clears the executed queries log
func (m *MockDatabase) ClearExecutedQueries() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executed = make([]string, 0)
}

// ExecContext simulates database execution
func (m *MockDatabase) ExecContext(ctx context.Context, query string, args ...interface{}) (MockResult, error) {
	// Simulate latency
	time.Sleep(m.latency)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Record executed query
	m.executed = append(m.executed, query)

	if m.shouldFail {
		return MockResult{}, m.failureError
	}

	// Basic simulation of different operations
	if contains(query, "INSERT") {
		return MockResult{lastInsertID: int64(m.getNextSequence("default")), rowsAffected: 1}, nil
	}
	if contains(query, "UPDATE") || contains(query, "DELETE") {
		return MockResult{rowsAffected: 1}, nil
	}

	return MockResult{rowsAffected: 0}, nil
}

// QueryContext simulates database queries
func (m *MockDatabase) QueryContext(ctx context.Context, query string, args ...interface{}) (*MockRows, error) {
	// Simulate latency
	time.Sleep(m.latency)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Record executed query
	m.executed = append(m.executed, query)

	if m.shouldFail {
		return nil, m.failureError
	}

	// Return mock rows based on query type
	return m.generateMockRows(query), nil
}

// QueryRowContext simulates single row queries
func (m *MockDatabase) QueryRowContext(ctx context.Context, query string, args ...interface{}) *MockRow {
	// Simulate latency
	time.Sleep(m.latency)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Record executed query
	m.executed = append(m.executed, query)

	if m.shouldFail {
		return &MockRow{err: m.failureError}
	}

	// Return mock row based on query
	return m.generateMockRow(query)
}

// PingContext simulates database ping
func (m *MockDatabase) PingContext(ctx context.Context) error {
	time.Sleep(m.latency)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.shouldFail {
		return m.failureError
	}

	return nil
}

// Close simulates database close
func (m *MockDatabase) Close() error {
	return nil
}

func (m *MockDatabase) getNextSequence(name string) int {
	m.sequences[name]++
	return m.sequences[name]
}

func (m *MockDatabase) generateMockRows(query string) *MockRows {
	rows := &MockRows{
		columns: []string{"id", "title", "description", "link", "published"},
		data:    make([][]interface{}, 0),
	}

	// Generate sample data based on query
	if contains(query, "feeds") {
		for i := 1; i <= 10; i++ {
			rows.data = append(rows.data, []interface{}{
				i,
				fmt.Sprintf("Feed Title %d", i),
				fmt.Sprintf("Feed Description %d", i),
				fmt.Sprintf("http://example.com/feed%d", i),
				time.Now().Add(-time.Duration(i) * time.Hour),
			})
		}
	}

	return rows
}

func (m *MockDatabase) generateMockRow(query string) *MockRow {
	if contains(query, "COUNT") {
		return &MockRow{
			value: int64(100),
		}
	}

	return &MockRow{
		value: "mock_value",
	}
}

// MockResult implements sql.Result
type MockResult struct {
	lastInsertID int64
	rowsAffected int64
}

func (r MockResult) LastInsertId() (int64, error) {
	return r.lastInsertID, nil
}

func (r MockResult) RowsAffected() (int64, error) {
	return r.rowsAffected, nil
}

// MockRows implements sql.Rows
type MockRows struct {
	columns []string
	data    [][]interface{}
	current int
}

func (r *MockRows) Columns() ([]string, error) {
	return r.columns, nil
}

func (r *MockRows) Close() error {
	return nil
}

func (r *MockRows) Next() bool {
	r.current++
	return r.current <= len(r.data)
}

func (r *MockRows) Scan(dest ...interface{}) error {
	if r.current <= 0 || r.current > len(r.data) {
		return fmt.Errorf("no rows available")
	}

	row := r.data[r.current-1]
	for i, dest := range dest {
		if i < len(row) {
			switch d := dest.(type) {
			case *int:
				if val, ok := row[i].(int); ok {
					*d = val
				}
			case *string:
				*d = fmt.Sprintf("%v", row[i])
			case *time.Time:
				if val, ok := row[i].(time.Time); ok {
					*d = val
				}
			}
		}
	}

	return nil
}

func (r *MockRows) Err() error {
	return nil
}

// MockRow implements sql.Row
type MockRow struct {
	value interface{}
	err   error
}

func (r *MockRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}

	if len(dest) > 0 {
		switch d := dest[0].(type) {
		case *int:
			if val, ok := r.value.(int64); ok {
				*d = int(val)
			}
		case *int64:
			if val, ok := r.value.(int64); ok {
				*d = val
			}
		case *string:
			*d = fmt.Sprintf("%v", r.value)
		}
	}

	return nil
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			indexOf(s, substr) >= 0)))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
