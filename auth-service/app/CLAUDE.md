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
1. **Red:** Write a failing test first
2. **Green:** Write minimal code to pass the test
3. **Refactor:** Improve code quality while keeping tests green

**Testing Strategy:**
- Test usecase and gateway layers primarily
- Mock external dependencies using gomock
- Use table-driven tests for comprehensive coverage
- Aim for >80% code coverage in tested layers

### Go 1.24+ Features to Use
- Enhanced type inference
- Range over integers
- Improved generics support
- Better error handling patterns

### Coding Standards

- Use `log/slog` for structured logging
- Follow Go idioms and effective Go patterns
- Use `gomock` for mocking
- Apply `gofmt` and `goimports` on every file change
- Use meaningful variable names
- Implement proper error wrapping with context

### Auth Service Specific Patterns

#### Domain Entity Example
```go
type User struct {
    ID              uuid.UUID
    KratosID        uuid.UUID
    TenantID        uuid.UUID
    Email           string
    Name            string
    Role            UserRole
    Status          UserStatus
    Preferences     UserPreferences
    CreatedAt       time.Time
    UpdatedAt       time.Time
    LastLoginAt     *time.Time
}
```

#### Usecase Pattern
```go
type AuthUsecase struct {
    authGateway  port.AuthGateway
    userGateway  port.UserGateway
    logger       *slog.Logger
}

func (u *AuthUsecase) InitiateLogin(ctx context.Context, req domain.LoginRequest) (*domain.LoginFlow, error) {
    // Business logic implementation
}
```

#### Error Handling Pattern
```go
if err != nil {
    u.logger.Error("login initiation failed",
        "error", err,
        "user_email", req.Email,
        "tenant_id", req.TenantID)
    return nil, fmt.Errorf("failed to initiate login: %w", err)
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