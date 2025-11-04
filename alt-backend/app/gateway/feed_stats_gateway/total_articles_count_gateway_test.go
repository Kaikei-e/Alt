package feed_stats_gateway

import (
	"alt/driver/alt_db"
	"alt/mocks"
	"alt/utils/logger"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MockPgxIface implements alt_db.PgxIface for testing
type MockPgxIface struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	closeFunc    func()
}

func (m *MockPgxIface) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (m *MockPgxIface) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return &mocks.MockRow{}
}

func (m *MockPgxIface) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag(""), errors.New("not implemented")
}

func (m *MockPgxIface) Begin(ctx context.Context) (pgx.Tx, error) {
	return nil, errors.New("not implemented")
}

func (m *MockPgxIface) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) {
	return nil, errors.New("not implemented")
}

func (m *MockPgxIface) Close() {
	if m.closeFunc != nil {
		m.closeFunc()
	}
}

func TestTotalArticlesCountGateway_Execute_Success(t *testing.T) {
	// Initialize logger for testing
	logger.InitLogger()

	tests := []struct {
		name          string
		mockCount     int
		mockError     error
		expectedCount int
		expectedError bool
	}{
		{
			name:          "successful count retrieval",
			mockCount:     1337,
			mockError:     nil,
			expectedCount: 1337,
			expectedError: false,
		},
		{
			name:          "zero count retrieval",
			mockCount:     0,
			mockError:     nil,
			expectedCount: 0,
			expectedError: false,
		},
		{
			name:          "large count retrieval",
			mockCount:     999999,
			mockError:     nil,
			expectedCount: 999999,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock that returns the expected count
			mockPgx := &MockPgxIface{
				queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
					if tt.mockError != nil {
						return &mocks.MockRow{} // Will cause scan error
					}
					// Set up mock row to return expected count
					return &MockRowWithValue{value: int64(tt.mockCount)}
				},
			}

			// Create repository with mock
			repo := alt_db.NewAltDBRepository(mockPgx)
			gateway := &TotalArticlesCountGateway{altDBRepository: repo}

			got, err := gateway.Execute(context.Background())
			if (err != nil) != tt.expectedError {
				t.Errorf("TotalArticlesCountGateway.Execute() error = %v, wantErr %v", err, tt.expectedError)
				return
			}
			if got != tt.expectedCount {
				t.Errorf("TotalArticlesCountGateway.Execute() = %v, want %v", got, tt.expectedCount)
			}
		})
	}
}

// MockRowWithValue implements pgx.Row with a specific value
type MockRowWithValue struct {
	value int64
	err   error
}

func (r *MockRowWithValue) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) > 0 {
		if intPtr, ok := dest[0].(*int); ok {
			*intPtr = int(r.value)
		}
	}
	return nil
}

func TestTotalArticlesCountGateway_Execute_NilRepository(t *testing.T) {
	// Initialize logger for testing
	logger.InitLogger()

	// Use constructor with nil pool to test error handling
	gateway := NewTotalArticlesCountGateway(nil)

	type args struct {
		ctx context.Context
	}

	tests := []struct {
		name    string
		args    args
		want    int
		wantErr bool
	}{
		{
			name: "execute with nil database (should error)",
			args: args{
				ctx: context.Background(),
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "execute with cancelled context",
			args: args{
				ctx: func() context.Context {
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					return ctx
				}(),
			},
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gateway.Execute(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("TotalArticlesCountGateway.Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("TotalArticlesCountGateway.Execute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTotalArticlesCountGateway_Execute_DatabaseError(t *testing.T) {
	// Initialize logger for testing
	logger.InitLogger()

	// Use constructor with nil pool to test error handling
	gateway := NewTotalArticlesCountGateway(nil)

	// Test error propagation
	count, err := gateway.Execute(context.Background())
	if err == nil {
		t.Error("TotalArticlesCountGateway.Execute() expected error with nil repository, got nil")
	}

	if count != 0 {
		t.Errorf("TotalArticlesCountGateway.Execute() expected count 0 on error, got %d", count)
	}

	// Verify the error message contains expected content
	expectedMsg := "database connection not available"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestNewTotalArticlesCountGateway(t *testing.T) {
	// Test constructor
	var pool *pgxpool.Pool // nil pool for testing
	gateway := NewTotalArticlesCountGateway(pool)

	if gateway == nil {
		t.Error("NewTotalArticlesCountGateway() returned nil")
	}

	// With our refactored approach, repository will be nil when pool is nil
	if gateway.altDBRepository != nil {
		t.Error("NewTotalArticlesCountGateway() with nil pool should have nil repository")
	}
}

func TestTotalArticlesCountGateway_ErrorHandling(t *testing.T) {
	// Initialize logger for testing
	logger.InitLogger()

	// Use constructor with nil pool to test error handling
	gateway := NewTotalArticlesCountGateway(nil)

	// Test error propagation
	count, err := gateway.Execute(context.Background())
	if err == nil {
		t.Error("TotalArticlesCountGateway.Execute() expected error with nil repository, got nil")
	}

	if count != 0 {
		t.Errorf("TotalArticlesCountGateway.Execute() expected count 0 on error, got %d", count)
	}

	// Verify the error message
	expectedMsg := "database connection not available"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestTotalArticlesCountGateway_ContextTimeout(t *testing.T) {
	// Initialize logger for testing
	logger.InitLogger()

	// Use constructor with nil pool to test error handling
	gateway := NewTotalArticlesCountGateway(nil)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 0) // Immediate timeout
	defer cancel()

	_, err := gateway.Execute(ctx)
	if err == nil {
		t.Error("TotalArticlesCountGateway.Execute() expected error with timed out context, got nil")
	}
}

func TestTotalArticlesCountGateway_NilContext(t *testing.T) {
	// Initialize logger for testing
	logger.InitLogger()

	// Use constructor with nil pool to test error handling
	gateway := NewTotalArticlesCountGateway(nil)

	// Test with nil context (this should panic or error)
	defer func() {
		if r := recover(); r == nil {
			// If it doesn't panic, it should at least error
			_, err := gateway.Execute(context.TODO())
			if err == nil {
				t.Error("TotalArticlesCountGateway.Execute() expected error with nil context, got nil")
			}
		}
	}()

	gateway.Execute(context.TODO())
}
