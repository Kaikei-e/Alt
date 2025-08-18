# CLAUDE.md - Auth Token Manager

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->
<!-- DO NOT use opus unless explicitly requested -->

## About Auth Token Manager

The Auth Token Manager is a security-critical microservice responsible for OAuth token lifecycle management in the Alt RSS reader ecosystem. This service handles authentication flows, token validation, renewal, and secure storage with enterprise-grade security requirements.

**Security Classification:** CRITICAL - Handles authentication tokens and sensitive user credentials.

## Architecture Overview

### Technology Stack
- **Runtime:** Deno 2.x with TypeScript
- **Security:** Web Crypto API, HMAC verification, structured logging
- **Deployment:** Kubernetes with security policies
- **Monitoring:** Structured JSON logging with sanitization

### Security-First Design Principles

1. **Zero Trust Architecture:** Assume all data could be compromised
2. **Defense in Depth:** Multiple security layers at every level
3. **Privacy by Design:** GDPR/CCPA compliance built-in
4. **Audit Everything:** Comprehensive tamper-evident logging
5. **Fail Securely:** Graceful degradation without data exposure

## Development Guidelines

### CRITICAL Security Requirements

#### 1. Logging Security (OWASP Top 10 #9)
**NEVER log sensitive information:**
- OAuth tokens (access_token, refresh_token, id_token)
- JWT payloads containing PII
- Client secrets and API keys
- User passwords or credentials
- Session IDs or correlation tokens
- Biometric data or health information
- Financial information (credit cards, bank details)
- Government identifiers (SSN, passport numbers)

#### 2. Data Sanitization Patterns (CWE-532 Prevention)
```typescript
// MANDATORY: Use StructuredLogger for all logging
import { logger, createComponentLogger } from './utils/logger.ts';

// GOOD: Sanitized logging
logger.info('OAuth flow completed', {
  user_id: sanitizedUserId,
  flow_type: 'authorization_code',
  duration_ms: 150
});

// BAD: Direct sensitive data logging
console.log('Token:', accessToken); // NEVER DO THIS
```

#### 3. Authentication Security Standards
- **Token Rotation:** Implement automatic refresh token rotation
- **Cryptographic Signatures:** HMAC-SHA256 for log integrity
- **Rate Limiting:** Implement OAuth flow rate limiting
- **Input Validation:** Strict validation of all OAuth parameters
- **CSRF Protection:** State parameter validation for OAuth flows

### Test-Driven Development for Security

#### Security Testing Approach
1. **Threat Modeling:** Identify attack vectors first
2. **Security Unit Tests:** Test sanitization, validation, encryption
3. **Integration Tests:** End-to-end OAuth flows with security validation
4. **Penetration Tests:** Automated security scanning in CI/CD

#### Required Test Coverage
- **Logging Sanitization:** 100% coverage of sensitive data patterns
- **OAuth Flows:** All standard and error scenarios
- **Input Validation:** Boundary conditions and injection attempts
- **Token Management:** Lifecycle, expiration, and rotation scenarios

### Code Quality Standards

#### TypeScript Security Patterns
```typescript
// Secure token handling
interface SecureTokenData {
  readonly tokenType: 'access' | 'refresh' | 'id';
  readonly expiresAt: Date;
  readonly scope: readonly string[];
  readonly subject: string;
}

// Input validation with security boundaries
function validateOAuthCode(code: unknown): string {
  if (typeof code !== 'string' || code.length < 10 || code.length > 512) {
    throw new SecurityError('Invalid authorization code format');
  }
  return code;
}
```

#### Error Handling with Security Focus
```typescript
try {
  const tokens = await exchangeCodeForTokens(code);
  logger.logAudit('token_exchange_success', { user_id: tokens.sub });
} catch (error) {
  // Log error without exposing sensitive details
  logger.logSecurity('token_exchange_failed', {
    error_type: error.name,
    timestamp: new Date().toISOString()
  });
  throw new PublicError('Authentication failed');
}
```

## Security Architecture

### Layer Security Model
```
┌─────────────────────────────────────────┐
│ API Layer (Rate Limiting + Input Valid) │
├─────────────────────────────────────────┤
│ Business Logic (OAuth Flow Management)  │
├─────────────────────────────────────────┤
│ Security Layer (Token Crypto + Audit)   │
├─────────────────────────────────────────┤
│ Data Layer (Encrypted Storage + Logs)   │
└─────────────────────────────────────────┘
```

### Cryptographic Standards
- **Token Encryption:** AES-256-GCM for token storage
- **Log Integrity:** HMAC-SHA256 for tamper detection
- **Key Management:** Web Crypto API with secure key derivation
- **Random Generation:** Cryptographically secure random for states/nonces

### Compliance Framework

#### GDPR Compliance
- **Data Minimization:** Log only necessary metadata
- **Right to Erasure:** Implement user data deletion
- **Consent Management:** Track consent for data processing
- **Data Protection Impact Assessment:** Regular security reviews

#### SOX Compliance (Financial Data)
- **Audit Trail:** Complete activity logging with integrity
- **Access Controls:** Role-based access to financial data
- **Change Management:** All code changes must be auditable
- **Data Retention:** Secure retention and disposal policies

#### CCPA Compliance
- **Consumer Rights:** Implement data access and deletion
- **Privacy Notices:** Clear data usage documentation
- **Opt-Out Mechanisms:** User control over data processing

## Implementation Patterns

### Secure OAuth Flow Implementation
```typescript
@logPerformance('oauth_authorization')
@logErrors('oauth')
export class OAuthFlowManager {
  async initiateFlow(clientId: string, scope: string[]): Promise<AuthFlowResult> {
    const logger = createComponentLogger('oauth-flow');
    const state = await this.generateSecureState();
    
    logger.logAudit('oauth_flow_initiated', {
      client_id: clientId,
      scopes: scope,
      state_hash: await this.hashState(state)
    });
    
    return {
      authUrl: this.buildAuthUrl(clientId, scope, state),
      state
    };
  }
}
```

### Secure Logging Patterns
```typescript
// Component-specific logger with context
const logger = createComponentLogger('token-validator');

// Audit logging with correlation
logger.logAudit('token_validation', {
  user_id: hashedUserId,
  token_type: 'access',
  validation_result: 'success',
  correlation_id: requestId
});

// Security event logging
logger.logSecurity('suspicious_token_usage', {
  threat_level: 'medium',
  indicators: ['unusual_geolocation', 'rapid_requests'],
  user_id: hashedUserId
});
```

### Performance with Security
```typescript
// Async sanitization for high-throughput logging
class AsyncDataSanitizer {
  private sanitizationCache = new LRUCache<string, string>(1000);
  
  async sanitize(data: unknown): Promise<unknown> {
    if (typeof data === 'string') {
      const cached = this.sanitizationCache.get(data);
      if (cached) return cached;
      
      const sanitized = await this.performSanitization(data);
      this.sanitizationCache.set(data, sanitized);
      return sanitized;
    }
    return this.sanitizeObject(data);
  }
}
```

## Security Monitoring

### Key Security Metrics
- **Failed Authentication Rate:** Monitor for brute force attempts
- **Token Usage Patterns:** Detect anomalous token usage
- **Log Integrity Checks:** Verify tamper-evident logging
- **Response Time Analysis:** Detect potential DoS attacks

### Incident Response
1. **Detection:** Automated security event correlation
2. **Containment:** Automatic token revocation for threats
3. **Analysis:** Forensic log analysis with preserved integrity
4. **Recovery:** Secure service restoration procedures

### Security Testing Requirements

#### Pre-deployment Security Checklist
- [ ] All sensitive data patterns covered in sanitization tests
- [ ] OAuth flow security validated with OWASP ZAP
- [ ] Log integrity verification implemented and tested
- [ ] Rate limiting effectiveness validated
- [ ] Input validation boundary testing completed
- [ ] Compliance requirements verified (GDPR/CCPA/SOX)

#### Continuous Security Monitoring
```bash
# Run security tests before deployment
deno test --allow-all tests/security/
deno task lint:security
deno task audit:dependencies
```

## Common Security Anti-Patterns to Avoid

### ❌ NEVER DO THESE:
```typescript
// DON'T: Log complete tokens
console.log('Access token:', token.access_token);

// DON'T: Store secrets in code
const CLIENT_SECRET = 'abc123-secret-key';

// DON'T: Use weak random generation
const state = Math.random().toString();

// DON'T: Skip input validation
const code = request.url.searchParams.get('code');
await exchangeCode(code); // Missing validation

// DON'T: Expose internal errors
throw new Error(`Database connection failed: ${dbError.message}`);
```

### ✅ ALWAYS DO THESE:
```typescript
// DO: Use structured logging with sanitization
logger.info('OAuth callback processed', { 
  flow_id: sanitizedFlowId,
  success: true 
});

// DO: Use environment variables for secrets
const CLIENT_SECRET = Deno.env.get('OAUTH_CLIENT_SECRET');

// DO: Use cryptographically secure random
const state = await crypto.subtle.generateKey({ name: 'HMAC', hash: 'SHA-256' });

// DO: Validate all inputs
const code = validateOAuthCode(request.url.searchParams.get('code'));

// DO: Use public error messages
throw new PublicError('Invalid request parameters');
```

## Development Workflow

### Security-First Development Process
1. **Threat Analysis:** Identify security requirements before coding
2. **Secure Design:** Design with security controls from the start
3. **Security Testing:** Write security tests before implementation
4. **Code Review:** Mandatory security review for all changes
5. **Security Validation:** Automated security scanning in CI/CD

### Local Development Security
```bash
# Setup secure development environment
deno task setup:security

# Run with security flags
deno run --allow-net --allow-env main.ts

# Security testing
deno task test:security
deno task audit:logs
```

## References and Resources

### Security Standards
- [OWASP Application Security Verification Standard](https://owasp.org/www-project-application-security-verification-standard/)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)
- [OAuth 2.1 Security Best Practices](https://tools.ietf.org/html/draft-ietf-oauth-security-topics)
- [OpenID Connect Security Guidelines](https://openid.net/specs/openid-connect-core-1_0.html)

### Compliance Resources
- [GDPR Developer Guidelines](https://gdpr.eu/developers/)
- [CCPA Compliance Guide](https://oag.ca.gov/privacy/ccpa)
- [SOX IT Controls](https://www.sox-online.com/it-controls/)

### Deno Security Documentation
- [Deno Security Model](https://docs.deno.com/runtime/fundamentals/security/)
- [Web Crypto API](https://docs.deno.com/runtime/web_crypto/)
- [Secure Headers](https://docs.deno.com/runtime/http_server_apis/)

---

**Remember:** Security is not optional. Every line of code in this service handles potentially sensitive authentication data. When in doubt, err on the side of caution and implement additional security measures.