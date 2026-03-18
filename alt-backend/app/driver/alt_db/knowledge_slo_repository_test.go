package alt_db

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAltDBRepository_GetProjectionLag_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	// Simulate 45.5 seconds of lag
	lagVal := 45.5
	rows := pgxmock.NewRows([]string{"lag_seconds"}).
		AddRow(&lagVal)

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT EXTRACT(EPOCH FROM (now() - MAX(updated_at))) FROM knowledge_projection_checkpoints`)).
		WillReturnRows(rows)

	lag, err := repo.GetProjectionLag(context.Background())
	require.NoError(t, err)
	assert.Equal(t, time.Duration(45500)*time.Millisecond, lag)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_GetProjectionLag_NoCheckpoints(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	// MAX(updated_at) returns NULL when table is empty, EXTRACT returns NULL
	rows := pgxmock.NewRows([]string{"lag_seconds"}).
		AddRow(nil)

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT EXTRACT(EPOCH FROM (now() - MAX(updated_at))) FROM knowledge_projection_checkpoints`)).
		WillReturnRows(rows)

	lag, err := repo.GetProjectionLag(context.Background())
	require.NoError(t, err)
	// When no checkpoints exist, lag should be -1 (sentinel) to indicate unknown
	assert.Equal(t, time.Duration(-1), lag)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_GetProjectionLag_QueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT EXTRACT(EPOCH FROM (now() - MAX(updated_at))) FROM knowledge_projection_checkpoints`)).
		WillReturnError(fmt.Errorf("connection refused"))

	_, err = repo.GetProjectionLag(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GetProjectionLag")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_GetProjectionAge_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	// Simulate 120.0 seconds of age
	ageVal := 120.0
	rows := pgxmock.NewRows([]string{"age_seconds"}).
		AddRow(&ageVal)

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT EXTRACT(EPOCH FROM (now() - MAX(updated_at))) FROM knowledge_projection_checkpoints`)).
		WillReturnRows(rows)

	age, err := repo.GetProjectionAge(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 120*time.Second, age)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_GetProjectionAge_NoCheckpoints(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	rows := pgxmock.NewRows([]string{"age_seconds"}).
		AddRow(nil)

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT EXTRACT(EPOCH FROM (now() - MAX(updated_at))) FROM knowledge_projection_checkpoints`)).
		WillReturnRows(rows)

	age, err := repo.GetProjectionAge(context.Background())
	require.NoError(t, err)
	assert.Equal(t, time.Duration(-1), age)
	require.NoError(t, mock.ExpectationsWereMet())
}
