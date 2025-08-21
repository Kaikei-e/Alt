# CLAUDE.md - Auth Service

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->
<!-- DO NOT use opus unless explicitly requested -->

## About this Auth Service

This is the authentication and authorization microservice for the Alt RSS reader project. It implements Ory Kratos integration, CSRF protection, session management, and multi-tenant support following Clean Architecture principles.

**Architecture:** Clean Architecture (5 layers) - REST → Usecase → Port → Gateway → Driver

## Tech Stack

### Core Technologies
- **Language:** Go 1.24+
- **Framework:** Echo v4
- **Database:** PostgreSQL (auth-postgres dedicated instance)
- **Identity Management:** Ory Kratos
- **Testing:** gomock, testify

### Features
- **Authentication Flow:** Registration, Login, Logout
- **Session Management:** Kratos session integration
- **CSRF Protection:** Session-based token management
- **User Management:** Profile and settings management
- **Tenant Management:** Multi-tenant support
- **Audit Logging:** Complete audit trail

## Architecture Design

### Clean Architecture Directory Layout
```
/auth-service
├─ app/
│  ├─ cmd/           # Application entry points
│  ├─ rest/          # HTTP handlers & routers
│  │  ├─ handlers/   # Route handlers
│  │  └─ middleware/ # HTTP middleware
│  ├─ usecase/       # Business logic orchestration
│  ├─ port/          # Interface definitions
│  ├─ gateway/       # Anti-corruption layer
│  ├─ driver/        # External integrations
│  │  ├─ postgres/   # Database drivers
│  │  └─ kratos/     # Ory Kratos integration
│  ├─ domain/        # Core entities & value objects
│  ├─ config/        # Configuration management
│  └─ utils/         # Cross-cutting concerns
│     ├─ logger/     # Structured logging
│     ├─ validator/  # Input validation
│     └─ errors/     # Error handling
└─ tests/            # Test suites
   ├─ unit/          # Unit tests
   ├─ integration/   # Integration tests
   └─ e2e/           # End-to-end tests
```

### Layer Responsibilities

| Layer | Purpose | Dependencies |
|-------|---------|--------------|
| **REST** | HTTP request/response mapping | → Usecase |
| **Usecase** | Business process orchestration | → Port |
| **Port** | Contract definitions | → Gateway |
| **Gateway** | External↔Domain translation | → Driver |
| **Driver** | Technical implementations | None |

## Development Guidelines

### Test-Driven Development (TDD)

**CRITICAL: Follow the Red-Green-Refactor cycle:**
1.  **Red**: Write a failing test that defines the new behavior.
2.  **Green**: Write the minimal code required to make the test pass.
3.  **Refactor**: Improve the code while keeping all tests green.

**Testing Strategy:**
- **Usecase & Gateway Layers**: These are the primary targets for unit tests. Mock all external dependencies using `gomock`.
- **Middleware Layer**: Test middleware logic in isolation using mocks and also with integration tests against a real Kratos instance.
- **Coverage**: Aim for >80% code coverage in tested layers.

### Testing Echo Middleware with Ory Kratos

Testing authentication middleware is critical. Here’s how to approach it.

#### 1. Unit Testing Middleware (with Mocks)

Unit tests should validate the middleware's logic without any network calls. We use `testify/mock` to create a mock of the Kratos client.

**Example Mock and Test:**
```go
// 1. Create a mock Kratos client
type MockKratosClient struct {
    mock.Mock
}

func (m *MockKratosClient) ToSession(cookie string, options ...kratos.FrontendApiToSessionOption) (*kratos.Session, *http.Response, error) {
    args := m.Called(cookie)
    // ... return mock session or error
}

// 2. Write a table-driven test for the middleware
func TestAuthMiddleware(t *testing.T) {
    // ... setup echo context ...

    mockKratos := new(MockKratosClient)
    middleware := AuthWithClient(mockKratos) // Your middleware constructor

    // Test Case: Valid Session
    mockKratos.On("ToSession", "valid_cookie").Return(&kratos.Session{}, &http.Response{StatusCode: http.StatusOK}, nil).Once()
    req.Header.Set("Cookie", "ory_kratos_session=valid_cookie")

    err := middleware(func(c echo.Context) error {
        return c.String(http.StatusOK, "test")
    })(c)

    assert.NoError(t, err)
    mockKratos.AssertExpectations(t)
    
    // ... add more test cases for invalid session, Kratos down, etc.
}
```

#### 2. Integration Testing Middleware

Integration tests verify that the middleware works with a real (but containerized) Ory Kratos instance.

**Strategy:**
- Use Docker Compose to run Ory Kratos and its database in your CI environment.
- Use the Kratos Go SDK to programmatically create test users and sessions before running tests.
- Use `httptest.Server` to run your Echo application and send real HTTP requests to it.

**Example Integration Test:**
```go
func TestAuthMiddleware_Integration(t *testing.T) {
    // Assumes Kratos is running at this address
    kratosURL := "http://127.0.0.1:4433"

    // 1. Setup: Create a test user and session in Kratos
    validSessionCookie := createTestSession(t, kratosURL)

    // 2. Setup Echo with the real middleware
    e := echo.New()
    kratosClient := newKratosClient(kratosURL) // Real Kratos client
    e.Use(AuthWithClient(kratosClient))
    e.GET("/protected", func(c echo.Context) error {
        return c.String(http.StatusOK, "welcome")
    })

    // 3. Execute request with valid cookie
    req := httptest.NewRequest(http.MethodGet, "/protected", nil)
    rec := httptest.NewRecorder()
    req.Header.Set("Cookie", "ory_kratos_session=" + validSessionCookie)
    e.ServeHTTP(rec, req)

    // 4. Assert
    assert.Equal(t, http.StatusOK, rec.Code)
}
```

## Security Considerations

### CSRF Protection
- Generate session-based CSRF tokens
- Validate tokens on state-changing operations
- Integrate with Kratos sessions for consistency

### Session Management
- Leverage Kratos session management
- Implement session validation middleware
- Handle session expiration gracefully

### Input Validation
- Validate all external inputs
- Sanitize user-provided data
- Implement rate limiting

### Audit Logging
- Log all authentication events
- Include relevant context (IP, user agent, etc.)
- Ensure compliance with audit requirements

## Configuration Management

### Environment Variables
```go
type Config struct {
    // Server
    Port     string `env:"PORT" default:"9500"`
    Host     string `env:"HOST" default:"0.0.0.0"`
    LogLevel string `env:"LOG_LEVEL" default:"info"`

    // Database
    DatabaseURL      string `env:"DATABASE_URL" required:"true"`
    DatabaseHost     string `env:"DB_HOST" default:"auth-postgres"`
    DatabasePort     string `env:"DB_PORT" default:"5432"`
    DatabaseName     string `env:"DB_NAME" default:"auth_db"`
    DatabaseUser     string `env:"DB_USER" default:"auth_user"`
    DatabasePassword string `env:"DB_PASSWORD" required:"true"`

    // Kratos
    KratosPublicURL string `env:"KRATOS_PUBLIC_URL" required:"true"`
    KratosAdminURL  string `env:"KRATOS_ADMIN_URL" required:"true"`

    // CSRF
    CSRFTokenLength int           `env:"CSRF_TOKEN_LENGTH" default:"32"`
    SessionTimeout  time.Duration `env:"SESSION_TIMEOUT" default:"24h"`

    // Features
    EnableAuditLog bool `env:"ENABLE_AUDIT_LOG" default:"true"`
    EnableMetrics  bool `env:"ENABLE_METRICS" default:"true"`
}
```

## Integration Points

### Alt-Backend Integration
- Provide session validation endpoints
- Supply user context for requests
- Handle CSRF token generation/validation

### Ory Kratos Integration
- Create/manage identity flows
- Session management
- User registration/authentication

### Database Integration
- User profile management
- Session tracking
- Audit logging
- Multi-tenant data isolation

## Testing Guidelines

### Unit Test Example
```go
func TestAuthUsecase_InitiateLogin(t *testing.T) {
    tests := []struct {
        name     string
        setup    func(*mocks.MockAuthGateway)
        input    domain.LoginRequest
        wantErr  bool
        validate func(*testing.T, *domain.LoginFlow, error)
    }{
        {
            name: "successful login initiation",
            setup: func(mockGateway *mocks.MockAuthGateway) {
                mockGateway.EXPECT().
                    CreateLoginFlow(gomock.Any()).
                    Return(&domain.LoginFlow{ID: "flow-123"}, nil)
            },
            input: domain.LoginRequest{Email: "test@example.com"},
            wantErr: false,
            validate: func(t *testing.T, flow *domain.LoginFlow, err error) {
                require.NoError(t, err)
                assert.Equal(t, "flow-123", flow.ID)
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

## Common Issues and Solutions

### Database Connection Management
- Use connection pooling appropriately
- Handle connection failures gracefully
- Implement health checks

### Kratos Integration Issues
- Verify Kratos configuration matches service expectations
- Handle Kratos service unavailability
- Implement proper retry mechanisms

### CSRF Token Management
- Ensure token generation is cryptographically secure
- Handle token expiration appropriately
- Sync with session lifecycle

## Performance Considerations

### Database Optimization
- Use appropriate indexes for auth queries
- Implement query result caching where appropriate
- Monitor slow queries

### Session Management
- Implement efficient session storage
- Clean up expired sessions regularly
- Use appropriate session timeout values

### Logging Performance
- Use structured logging efficiently
- Implement log levels appropriately
- Avoid excessive logging in hot paths

## Important Reminders

- **TDD First:** Always write tests before implementation
- **Security Focus:** Authentication service requires extra security attention
- **Clean Architecture:** Maintain clear layer boundaries
- **Error Handling:** Comprehensive error handling with context
- **Logging:** Structured logging for observability
- **Multi-tenant:** Design all features with multi-tenancy in mind

## References

- [Ory Kratos Documentation](https://www.ory.sh/docs/kratos/)
- [PostgreSQL Best Practices](https://wiki.postgresql.org/wiki/Don't_Do_This)
- [Go 1.24 Release Notes](https://golang.org/doc/go1.24)
- [Echo Framework](https://echo.labstack.com/)
- [Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)