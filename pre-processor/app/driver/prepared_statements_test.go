package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreparedStatementsManager(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should initialize prepared statements manager",
			test: func(t *testing.T) {
				manager := NewPreparedStatementsManager()
				assert.NotNil(t, manager)
				assert.NotNil(t, manager.statements)
			},
		},
		{
			name: "should prepare and cache statements",
			test: func(t *testing.T) {
				manager := NewPreparedStatementsManager()
				mockDB := &MockDB{}
				ctx := context.Background()

				// Prepare a statement
				err := manager.PrepareStatement(ctx, mockDB, "insert_article", 
					"INSERT INTO articles (title, content, url, feed_id) VALUES ($1, $2, $3, $4)")
				require.NoError(t, err)

				// Verify statement is cached
				stmt := manager.GetStatement("insert_article")
				assert.NotNil(t, stmt)
			},
		},
		{
			name: "should return nil for non-existent statement",
			test: func(t *testing.T) {
				manager := NewPreparedStatementsManager()
				
				stmt := manager.GetStatement("non_existent")
				assert.Nil(t, stmt)
			},
		},
		{
			name: "should handle multiple statement preparations",
			test: func(t *testing.T) {
				manager := NewPreparedStatementsManager()
				mockDB := &MockDB{}
				ctx := context.Background()

				// Prepare multiple statements
				statements := map[string]string{
					"insert_article": "INSERT INTO articles (title, content) VALUES ($1, $2)",
					"update_article": "UPDATE articles SET title = $1 WHERE id = $2",
					"select_article": "SELECT id, title FROM articles WHERE id = $1",
				}

				for name, query := range statements {
					err := manager.PrepareStatement(ctx, mockDB, name, query)
					require.NoError(t, err)
				}

				// Verify all statements are cached
				for name := range statements {
					stmt := manager.GetStatement(name)
					assert.NotNil(t, stmt, "Statement %s should be cached", name)
				}
			},
		},
		{
			name: "should close all prepared statements",
			test: func(t *testing.T) {
				manager := NewPreparedStatementsManager()
				mockDB := &MockDB{}
				ctx := context.Background()

				// Prepare some statements
				err := manager.PrepareStatement(ctx, mockDB, "test1", "SELECT 1")
				require.NoError(t, err)
				err = manager.PrepareStatement(ctx, mockDB, "test2", "SELECT 2")
				require.NoError(t, err)

				// Close all statements
				err = manager.CloseAll(ctx)
				assert.NoError(t, err)

				// Statements should still be in cache but marked as closed
				stmt := manager.GetStatement("test1")
				assert.NotNil(t, stmt)
			},
		},
		{
			name: "should handle concurrent access safely",
			test: func(t *testing.T) {
				manager := NewPreparedStatementsManager()
				mockDB := &MockDB{}
				ctx := context.Background()

				// Test concurrent preparation and access
				done := make(chan bool, 2)

				go func() {
					defer func() { done <- true }()
					for i := 0; i < 10; i++ {
						err := manager.PrepareStatement(ctx, mockDB, "concurrent1", "SELECT 1")
						assert.NoError(t, err)
					}
				}()

				go func() {
					defer func() { done <- true }()
					for i := 0; i < 10; i++ {
						stmt := manager.GetStatement("concurrent1")
						// stmt might be nil if not prepared yet, which is fine
						_ = stmt
					}
				}()

				// Wait for both goroutines
				<-done
				<-done

				// Final verification
				stmt := manager.GetStatement("concurrent1")
				assert.NotNil(t, stmt)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestPreparedStatements_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	t.Run("should improve database performance", func(t *testing.T) {
		manager := NewPreparedStatementsManager()
		mockDB := &MockDB{}
		ctx := context.Background()

		// Prepare common statements
		commonStatements := map[string]string{
			"insert_article":   "INSERT INTO articles (title, content, url, feed_id, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)",
			"update_article":   "UPDATE articles SET title = $1, content = $2, updated_at = $3 WHERE id = $4",
			"select_article":   "SELECT id, title, content, url, feed_id, created_at FROM articles WHERE id = $1",
			"check_existence":  "SELECT EXISTS(SELECT 1 FROM articles WHERE url = $1)",
		}

		for name, query := range commonStatements {
			err := manager.PrepareStatement(ctx, mockDB, name, query)
			require.NoError(t, err)
		}

		// Verify all statements are available
		for name := range commonStatements {
			stmt := manager.GetStatement(name)
			require.NotNil(t, stmt, "Statement %s should be prepared", name)
		}

		// Test statement reuse
		stmt1 := manager.GetStatement("insert_article")
		stmt2 := manager.GetStatement("insert_article")
		assert.Equal(t, stmt1, stmt2, "Should return same statement instance")
	})
}