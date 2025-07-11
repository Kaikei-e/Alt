package driver

import (
	"context"
)

// MockDB implements database interface for testing
type MockDB struct{}

func (m *MockDB) BeginTx(ctx context.Context, opts interface{}) (interface{}, error) {
	return &MockTx{}, nil
}

func (m *MockDB) Exec(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
	return nil, nil
}

type MockTx struct{}

func (m *MockTx) Exec(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
	return nil, nil
}

func (m *MockTx) Commit(ctx context.Context) error {
	return nil
}

func (m *MockTx) Rollback(ctx context.Context) error {
	return nil
}
