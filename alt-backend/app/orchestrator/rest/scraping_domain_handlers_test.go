package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/di"
	"alt/domain"
	middleware_custom "alt/middleware"
	"alt/orchestrator/usecase/scraping_domain_usecase"
	"alt/utils/logger"
)

const (
	testAdminAuthSecret   = "test-admin-jwt-secret-32-bytes!!"
	testAdminAuthIssuer   = "auth-hub"
	testAdminAuthAudience = "alt-backend"
)

type stubScrapingDomainPort struct {
	domains       []*domain.ScrapingDomain
	byID          map[uuid.UUID]*domain.ScrapingDomain
	saved         []*domain.ScrapingDomain
	updatedID     uuid.UUID
	updatedPolicy *domain.ScrapingPolicyUpdate
	err           error
}

func (s *stubScrapingDomainPort) GetByDomain(_ context.Context, _ string) (*domain.ScrapingDomain, error) {
	return nil, s.err
}

func (s *stubScrapingDomainPort) GetByID(_ context.Context, id uuid.UUID) (*domain.ScrapingDomain, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.byID != nil {
		return s.byID[id], nil
	}
	return nil, nil
}

func (s *stubScrapingDomainPort) Save(_ context.Context, d *domain.ScrapingDomain) error {
	s.saved = append(s.saved, d)
	return s.err
}

func (s *stubScrapingDomainPort) List(_ context.Context, _, _ int) ([]*domain.ScrapingDomain, error) {
	return s.domains, s.err
}

func (s *stubScrapingDomainPort) UpdatePolicy(_ context.Context, id uuid.UUID, update *domain.ScrapingPolicyUpdate) error {
	s.updatedID = id
	s.updatedPolicy = update
	return s.err
}

type stubRobotsTxtFetcherPort struct {
	robots *domain.RobotsTxt
	err    error
}

func (s *stubRobotsTxtFetcherPort) FetchRobotsTxt(_ context.Context, _, _ string) (*domain.RobotsTxt, error) {
	return s.robots, s.err
}

func testAuthCfg() config.AuthConfig {
	return config.AuthConfig{
		BackendTokenSecret:   testAdminAuthSecret,
		BackendTokenIssuer:   testAdminAuthIssuer,
		BackendTokenAudience: testAdminAuthAudience,
	}
}

func signRESTTestToken(t *testing.T, role domain.UserRole) string {
	t.Helper()
	uid := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	claims := &middleware_custom.BackendClaims{
		Email:    "user@example.com",
		Role:     string(role),
		Sid:      "sess-1",
		TenantID: uid.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uid.String(),
			Issuer:    testAdminAuthIssuer,
			Audience:  jwt.ClaimStrings{testAdminAuthAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testAdminAuthSecret))
	require.NoError(t, err)
	return signed
}

func injectUserContextMiddleware(role domain.UserRole) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			uid := uuid.MustParse("11111111-2222-3333-4444-555555555555")
			ctx := domain.SetUserContext(c.Request().Context(), &domain.UserContext{
				UserID:    uid,
				TenantID:  uid,
				Email:     "user@example.com",
				Role:      role,
				LoginAt:   time.Now().UTC(),
				ExpiresAt: time.Now().UTC().Add(time.Hour),
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

func scrapingDomainRoutesTable(domainID uuid.UUID) []struct {
	name   string
	method string
	target string
	body   string
} {
	return []struct {
		name   string
		method string
		target string
		body   string
	}{
		{
			name:   "GET /v1/admin/scraping-domains",
			method: http.MethodGet,
			target: "/v1/admin/scraping-domains",
		},
		{
			name:   "GET /v1/admin/scraping-domains/:id",
			method: http.MethodGet,
			target: "/v1/admin/scraping-domains/" + domainID.String(),
		},
		{
			name:   "PATCH /v1/admin/scraping-domains/:id",
			method: http.MethodPatch,
			target: "/v1/admin/scraping-domains/" + domainID.String(),
			body:   `{"allow_fetch_body":false}`,
		},
		{
			name:   "POST /v1/admin/scraping-domains/:id/refresh-robots",
			method: http.MethodPost,
			target: "/v1/admin/scraping-domains/" + domainID.String() + "/refresh-robots",
		},
	}
}

func TestScrapingDomainRoutes_Unauthenticated_Returns401(t *testing.T) {
	logger.InitLogger()
	domainID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	for _, rt := range scrapingDomainRoutesTable(domainID) {
		t.Run(rt.name, func(t *testing.T) {
			e := echo.New()
			e.Use(echomiddleware.Recover())
			v1 := e.Group("/v1")
			registerScrapingDomainRoutes(v1, &di.ApplicationComponents{}, testAuthCfg())

			req := httptest.NewRequest(rt.method, rt.target, strings.NewReader(rt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

func TestScrapingDomainRoutes_NonAdminUserContext_Returns403(t *testing.T) {
	logger.InitLogger()
	domainID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	userToken := signRESTTestToken(t, domain.UserRoleUser)

	for _, rt := range scrapingDomainRoutesTable(domainID) {
		t.Run(rt.name+" via JWT", func(t *testing.T) {
			e := echo.New()
			e.Use(echomiddleware.Recover())
			v1 := e.Group("/v1")
			registerScrapingDomainRoutes(v1, &di.ApplicationComponents{}, testAuthCfg())

			req := httptest.NewRequest(rt.method, rt.target, strings.NewReader(rt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req.Header.Set("X-Alt-Backend-Token", userToken)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusForbidden, rec.Code)
		})

		t.Run(rt.name+" via domain.UserContext", func(t *testing.T) {
			e := echo.New()
			authMW := middleware_custom.NewAuthMiddleware(logger.Logger, testAuthCfg())
			admin := e.Group("/v1/admin/scraping-domains", injectUserContextMiddleware(domain.UserRoleUser), authMW.RequireAdmin())
			admin.GET("", handleListScrapingDomains(&di.ApplicationComponents{}))
			admin.GET("/:id", handleGetScrapingDomain(&di.ApplicationComponents{}))
			admin.PATCH("/:id", handleUpdateScrapingDomainPolicy(&di.ApplicationComponents{}))
			admin.POST("/:id/refresh-robots", handleRefreshRobotsTxt(&di.ApplicationComponents{}))

			req := httptest.NewRequest(rt.method, rt.target, strings.NewReader(rt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusForbidden, rec.Code)
		})
	}
}

func TestScrapingDomainRoutes_AdminRole_SucceedsAcrossAll4Routes(t *testing.T) {
	logger.InitLogger()
	domainID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	sampleDomain := &domain.ScrapingDomain{
		ID:                  domainID,
		Domain:              "example.com",
		Scheme:              "https",
		AllowFetchBody:      true,
		AllowMLTraining:     true,
		AllowCacheDays:      7,
		ForceRespectRobots:  true,
		RobotsDisallowPaths: []string{"/private"},
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	port := &stubScrapingDomainPort{
		domains: []*domain.ScrapingDomain{sampleDomain},
		byID: map[uuid.UUID]*domain.ScrapingDomain{
			domainID: sampleDomain,
		},
	}
	robotsPort := &stubRobotsTxtFetcherPort{
		robots: &domain.RobotsTxt{
			URL:           "https://example.com/robots.txt",
			Content:       "User-agent: *\nDisallow: /admin",
			StatusCode:    200,
			CrawlDelay:    5,
			DisallowPaths: []string{"/admin"},
		},
	}
	uc := scraping_domain_usecase.NewScrapingDomainUsecaseWithRobotsTxt(port, robotsPort)
	container := &di.ApplicationComponents{
		ScrapingDomainUsecase: uc,
	}

	e := echo.New()
	v1 := e.Group("/v1")
	registerScrapingDomainRoutes(v1, container, testAuthCfg())
	adminToken := signRESTTestToken(t, domain.UserRoleAdmin)

	t.Run("GET /v1/admin/scraping-domains lists domains", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/scraping-domains?limit=10&offset=0", nil)
		req.Header.Set("X-Alt-Backend-Token", adminToken)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var got []ScrapingDomainResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Len(t, got, 1)
		assert.Equal(t, domainID.String(), got[0].ID)
		assert.Equal(t, "example.com", got[0].Domain)
	})

	t.Run("GET /v1/admin/scraping-domains/:id gets single domain", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/scraping-domains/"+domainID.String(), nil)
		req.Header.Set("X-Alt-Backend-Token", adminToken)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var got ScrapingDomainResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, domainID.String(), got.ID)
		assert.Equal(t, "example.com", got.Domain)
	})

	t.Run("PATCH /v1/admin/scraping-domains/:id updates policy", func(t *testing.T) {
		body := bytes.NewBufferString(`{"allow_fetch_body":false,"allow_cache_days":14}`)
		req := httptest.NewRequest(http.MethodPatch, "/v1/admin/scraping-domains/"+domainID.String(), body)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set("X-Alt-Backend-Token", adminToken)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, domainID, port.updatedID)
		require.NotNil(t, port.updatedPolicy)
		require.NotNil(t, port.updatedPolicy.AllowFetchBody)
		assert.False(t, *port.updatedPolicy.AllowFetchBody)
		require.NotNil(t, port.updatedPolicy.AllowCacheDays)
		assert.Equal(t, 14, *port.updatedPolicy.AllowCacheDays)
	})

	t.Run("POST /v1/admin/scraping-domains/:id/refresh-robots refreshes robots.txt", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/scraping-domains/"+domainID.String()+"/refresh-robots", nil)
		req.Header.Set("X-Alt-Backend-Token", adminToken)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Len(t, port.saved, 1)
		assert.Equal(t, []string{"/admin"}, port.saved[0].RobotsDisallowPaths)
	})
}
