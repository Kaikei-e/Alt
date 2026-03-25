package alt_db

import (
	"alt/domain"
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

var saveScrapingDomainQueryRe = regexp.MustCompile(`INSERT INTO scraping_domains`)

func TestSaveScrapingDomain_JSONBParam(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ScrapingRepository{pool: mock}

	sd := &domain.ScrapingDomain{
		ID:                  uuid.New(),
		Domain:              "example.com",
		Scheme:              "https",
		RobotsDisallowPaths: []string{"/admin/"},
	}

	// pgxmock's WithArgs checks types: string != []byte.
	// $13 (robots_disallow_paths) must arrive as string, not []byte.
	mock.ExpectExec(saveScrapingDomainQueryRe.String()).
		WithArgs(
			sd.ID, sd.Domain, sd.Scheme,
			sd.AllowFetchBody, sd.AllowMLTraining, sd.AllowCacheDays,
			sd.ForceRespectRobots, sd.RobotsTxtURL, sd.RobotsTxtContent,
			sd.RobotsTxtFetchedAt, sd.RobotsTxtLastStatus, sd.RobotsCrawlDelaySec,
			string(`["/admin/"]`), pgxmock.AnyArg(), pgxmock.AnyArg(),
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.SaveScrapingDomain(context.Background(), sd)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveScrapingDomain_EmptyPaths(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ScrapingRepository{pool: mock}

	sd := &domain.ScrapingDomain{
		ID:                  uuid.New(),
		Domain:              "empty.com",
		Scheme:              "https",
		RobotsDisallowPaths: []string{},
	}

	mock.ExpectExec(saveScrapingDomainQueryRe.String()).
		WithArgs(
			sd.ID, sd.Domain, sd.Scheme,
			sd.AllowFetchBody, sd.AllowMLTraining, sd.AllowCacheDays,
			sd.ForceRespectRobots, sd.RobotsTxtURL, sd.RobotsTxtContent,
			sd.RobotsTxtFetchedAt, sd.RobotsTxtLastStatus, sd.RobotsCrawlDelaySec,
			string("[]"), pgxmock.AnyArg(), pgxmock.AnyArg(),
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.SaveScrapingDomain(context.Background(), sd)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveScrapingDomain_NilPaths(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ScrapingRepository{pool: mock}

	sd := &domain.ScrapingDomain{
		ID:                  uuid.New(),
		Domain:              "nil.com",
		Scheme:              "https",
		RobotsDisallowPaths: nil,
	}

	// nil paths should be normalized to "[]" (string), not "null"
	mock.ExpectExec(saveScrapingDomainQueryRe.String()).
		WithArgs(
			sd.ID, sd.Domain, sd.Scheme,
			sd.AllowFetchBody, sd.AllowMLTraining, sd.AllowCacheDays,
			sd.ForceRespectRobots, sd.RobotsTxtURL, sd.RobotsTxtContent,
			sd.RobotsTxtFetchedAt, sd.RobotsTxtLastStatus, sd.RobotsCrawlDelaySec,
			string("[]"), pgxmock.AnyArg(), pgxmock.AnyArg(),
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.SaveScrapingDomain(context.Background(), sd)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveScrapingDomain_DBError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ScrapingRepository{pool: mock}

	sd := &domain.ScrapingDomain{
		ID:                  uuid.New(),
		Domain:              "error.com",
		Scheme:              "https",
		RobotsDisallowPaths: []string{},
	}

	mock.ExpectExec(saveScrapingDomainQueryRe.String()).
		WithArgs(
			sd.ID, sd.Domain, sd.Scheme,
			sd.AllowFetchBody, sd.AllowMLTraining, sd.AllowCacheDays,
			sd.ForceRespectRobots, sd.RobotsTxtURL, sd.RobotsTxtContent,
			sd.RobotsTxtFetchedAt, sd.RobotsTxtLastStatus, sd.RobotsCrawlDelaySec,
			string("[]"), pgxmock.AnyArg(), pgxmock.AnyArg(),
		).
		WillReturnError(errors.New("connection refused"))

	err = repo.SaveScrapingDomain(context.Background(), sd)
	require.Error(t, err)
	require.ErrorContains(t, err, "error saving scraping domain")
	require.NoError(t, mock.ExpectationsWereMet())
}
