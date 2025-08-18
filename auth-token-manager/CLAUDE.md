# CLAUDE.md - Auth Token Manager

## About This Service

A simple OAuth token refresh service for Inoreader API integration in the Alt RSS Reader project. This service handles OAuth token lifecycle management with basic security practices.

## Technology Stack

- **Runtime**: Deno 2.x with TypeScript
- **Purpose**: Inoreader OAuth token refresh and management
- **Architecture**: Simple microservice with basic security

## Key Security Requirement

**Critical**: Never log OAuth tokens or API credentials in plain text (CWE-532 prevention).

## Development Guidelines

### Secure Logging Practices

```typescript
import { logger } from './utils/logger.ts';

// ✅ Good: Using sanitized logging
logger.info('OAuth token refreshed', {
  user_id: 'user123',
  expires_in: 3600,
  // access_token is automatically sanitized
});

// ❌ Bad: Direct token logging
console.log('Token:', accessToken); // Never do this
```

### Basic Security Rules

1. **No plaintext tokens**: All OAuth tokens are automatically sanitized in logs
2. **Environment-based debugging**: Debug info only shows in development
3. **Input validation**: Basic size and format checks for API calls
4. **Error handling**: Don't expose internal errors to external callers

### Code Structure

```
src/
├── auth/           # OAuth flow management
├── utils/
│   └── logger.ts   # Sanitized logging utilities
└── k8s/            # Kubernetes secret management
```

## Logging System

### DataSanitizer

Automatically removes OAuth tokens from log output:

- `access_token`, `refresh_token` → `[REDACTED]`
- Bearer tokens → First/last 4 chars + `[REDACTED]`
- Inoreader `AppId`, `AppKey` → `[REDACTED]`

### Logger Usage

```typescript
import { logger } from './utils/logger.ts';

// All these will be automatically sanitized
logger.info('Processing request', {
  access_token: token,    // → [REDACTED]
  user_id: userId,        // → preserved
  status: 'success'       // → preserved
});
```

## Testing

Run security tests to verify token sanitization:

```bash
deno test --allow-all tests/security/
```

## Configuration

Set environment variables:

```bash
# Required
INOREADER_CLIENT_ID=your_client_id
INOREADER_CLIENT_SECRET=your_client_secret

# Optional
LOG_LEVEL=INFO              # DEBUG, INFO, WARN, ERROR
NODE_ENV=production         # development, production
```

## Security Best Practices

### Do's ✅
- Use the structured logger for all output
- Keep tokens in environment variables
- Validate input sizes and formats
- Handle errors gracefully

### Don'ts ❌
- Don't use `console.log()` for tokens
- Don't hardcode credentials
- Don't expose internal error details
- Don't log full request/response bodies without sanitization

## Common Patterns

### OAuth Token Refresh

```typescript
async function refreshToken(refreshToken: string) {
  logger.info('Starting token refresh');
  
  try {
    const response = await fetch(INOREADER_TOKEN_URL, {
      method: 'POST',
      body: new URLSearchParams({
        refresh_token: refreshToken,
        grant_type: 'refresh_token'
      })
    });
    
    const tokens = await response.json();
    
    // This will be sanitized in logs
    logger.info('Token refresh successful', {
      access_token: tokens.access_token,
      expires_in: tokens.expires_in
    });
    
    return tokens;
  } catch (error) {
    logger.error('Token refresh failed', { error: error.message });
    throw new Error('Token refresh failed');
  }
}
```

### Error Handling

```typescript
// Good: Safe error logging
logger.error('OAuth request failed', {
  error: 'invalid_grant',
  status_code: 400
});

// Bad: Exposing sensitive data
logger.error('OAuth failed', { response: fullResponse });
```

## Deployment

This service runs in Kubernetes and updates secrets automatically:

```bash
# Deploy to development
kubectl apply -f k8s/

# Check logs (tokens will be sanitized)
kubectl logs -f deployment/auth-token-manager
```

## Troubleshooting

### Common Issues

1. **Tokens in logs**: Check that you're using `logger` instead of `console.log`
2. **Config missing**: Verify environment variables are set
3. **Network errors**: Check Inoreader API connectivity

### Debug Mode

```bash
# Enable debug logging (development only)
NODE_ENV=development LOG_LEVEL=DEBUG deno run main.ts
```

## Maintenance

- Review logs monthly for any sanitization gaps
- Update token patterns if new API formats are introduced
- Keep dependencies updated for security patches

---

**Remember**: This is a simple OAuth service. Keep it simple, secure, and focused on its core purpose.