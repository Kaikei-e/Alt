// k6/helpers/auth.js - Authentication header construction for K6 load tests
//
// Uses X-Alt-Backend-Token (JWT) authentication
// (see alt-backend/app/middleware/auth_middleware.go).

import { getConfig } from "./config.js";
import { decodeJWT } from "./jwt.js";

const UUID_REGEX = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
const MAX_JWT_LENGTH = 4096;
const MAX_TTL_SECONDS = 300; // 5 minutes max token validity window
const CLOCK_SKEW_SECONDS = 60; // Allow 60s clock skew between AuthHub issuer and runner

/**
 * Validates an issued AuthHub JWT token according to security requirements:
 * - Length under 4096 bytes
 * - Valid 3-part JWT format
 * - iat present, numeric, and not in the future (allowing clock skew up to 60s)
 * - exp present, numeric, and exp > iat
 * - Token validity window (exp - iat) <= 300 seconds (5 minutes max)
 * - Request-time expiry check: exp > now (fail-closed)
 * - If minRemainingSeconds or cohortDuration configured:
 *     remaining validity (exp - now) >= minRemainingSeconds (or cohortDuration + 30s)
 * - iss === "auth-hub"
 * - aud contains "alt-backend"
 * - sub matches valid UUID format (and matches testUserId if configured)
 * - tenant_id matches valid UUID format (and matches testTenantId if configured)
 * - role is not "admin" or "wildcard"
 * - does NOT require invented scope claim (AuthHub does not emit scope)
 *
 * @param {string} token - JWT string to validate
 * @param {object} [cfg] - Optional config object (defaults to getConfig())
 * @param {object} [options] - Optional validation options (minRemainingSeconds, cohortDuration)
 * @returns {object} Validated claims
 *
 * @note Fixture tokens loaded from per-user SharedArray and passed as `tokenOverride`
 *   to `getAuthHeaders()` bypass the default-token remaining-budget path. Those tokens
 *   MUST be validated at INIT TIME via `validateFixtureTokenAtInit(token, cfg)` before
 *   the load-test run begins to enforce the same 180s remaining-budget guard.
 */
export function validateAuthHubToken(token, cfg = getConfig(), options = {}) {
  if (!token) {
    throw new Error("Missing K6_API_TOKEN / auth token");
  }

  if (typeof token !== "string" || token.length > MAX_JWT_LENGTH) {
    throw new Error(`Token exceeds maximum allowed size (${MAX_JWT_LENGTH} bytes)`);
  }

  const claims = decodeJWT(token);
  if (!claims || typeof claims !== "object") {
    throw new Error("Invalid JWT token format");
  }

  const now = Math.floor(Date.now() / 1000);

  // Validate iat (Issued At)
  if (typeof claims.iat !== "number" || isNaN(claims.iat)) {
    throw new Error("Token missing or invalid iat claim");
  }
  if (claims.iat > now + CLOCK_SKEW_SECONDS) {
    throw new Error(`Token iat is in the future. iat=${claims.iat} now=${now} skew=${CLOCK_SKEW_SECONDS}`);
  }

  // Validate exp (Expiration Time)
  if (typeof claims.exp !== "number" || isNaN(claims.exp)) {
    throw new Error("Token missing or invalid exp claim");
  }

  // Request-timed expiry check (fail closed immediately when expired)
  if (claims.exp <= now) {
    throw new Error(`Token expired. exp=${claims.exp} now=${now}`);
  }

  if (claims.exp <= claims.iat) {
    throw new Error(`Token exp must be greater than iat. exp=${claims.exp} iat=${claims.iat}`);
  }

  // Enforce maximum token validity window (exp - iat <= 300 seconds)
  if (claims.exp - claims.iat > MAX_TTL_SECONDS) {
    throw new Error(`Token validity duration (exp - iat) exceeds maximum allowed 5 minutes (${MAX_TTL_SECONDS}s). exp=${claims.exp} iat=${claims.iat} diff=${claims.exp - claims.iat}`);
  }

  // Token remaining validity from current time must not exceed 5 minutes + clock skew
  if (claims.exp > now + MAX_TTL_SECONDS + CLOCK_SKEW_SECONDS) {
    throw new Error(`Token lifetime/expiration exceeds maximum allowed 5 minutes. exp=${claims.exp} maxAllowed=${now + MAX_TTL_SECONDS}`);
  }

  // Bounded cohort remaining validity check at init/request time
  const remaining = claims.exp - now;
  const requiredRemaining = options.minRemainingSeconds !== undefined
    ? options.minRemainingSeconds
    : (options.cohortDuration ? options.cohortDuration + 30 : 0);

  if (requiredRemaining > 0 && remaining < requiredRemaining) {
    throw new Error(`Token remaining validity (${remaining}s) is insufficient (required >= ${requiredRemaining}s)`);
  }

  // Validate Issuer
  if (claims.iss !== "auth-hub") {
    throw new Error(`Token iss must be auth-hub, got ${claims.iss}`);
  }

  // Validate Audience (can be string or array in JWT)
  const auds = Array.isArray(claims.aud) ? claims.aud : [claims.aud];
  if (!auds.includes("alt-backend")) {
    throw new Error(`Token aud must include alt-backend, got ${JSON.stringify(claims.aud)}`);
  }

  // Validate Subject (UserID)
  if (!claims.sub || typeof claims.sub !== "string" || !UUID_REGEX.test(claims.sub)) {
    throw new Error(`Token sub claim must be a valid UUID, got ${claims.sub}`);
  }
  if (cfg.testUserId && claims.sub !== cfg.testUserId) {
    throw new Error(`Token sub claim mismatch. expected=${cfg.testUserId} got=${claims.sub}`);
  }

  // Validate Tenant (tenant_id)
  // Single-tenant AuthHub sets tenant_id == sub; multi-tenant carries tenant_id
  const tenantId = claims.tenant_id || claims.tenant;
  if (tenantId) {
    if (!UUID_REGEX.test(tenantId)) {
      throw new Error(`Token tenant_id claim must be a valid UUID, got ${tenantId}`);
    }
    if (cfg.testTenantId && tenantId !== cfg.testTenantId) {
      throw new Error(`Token tenant_id claim mismatch. expected=${cfg.testTenantId} got=${tenantId}`);
    }
  } else if (cfg.testTenantId) {
    throw new Error(`Token missing tenant_id claim while testTenantId is configured`);
  }

  // Validate Role: admin and wildcard roles forbidden for load test scope
  if (claims.role === "admin" || claims.role === "wildcard") {
    throw new Error("Admin/wildcard roles are not permitted for load test scope");
  }

  // Note: Production AuthHub does not emit a 'scope' claim, so we do not enforce one.

  return claims;
}

/**
 * Validates an issued token for use in a bounded load test cohort at init time.
 * Enforces that remaining token validity >= cohortDuration + 30 seconds buffer.
 *
 * @param {string} token - JWT string to validate
 * @param {number} [cohortDuration=150] - Cohort duration in seconds
 * @param {object} [cfg=getConfig()] - Config object
 * @returns {object} Validated claims
 */
export function validateCohortTokenAtInit(token, cohortDuration = 150, cfg = getConfig()) {
  return validateAuthHubToken(token, cfg, {
    minRemainingSeconds: cohortDuration + 30,
    cohortDuration: cohortDuration,
  });
}

/**
 * Validates a per-user SharedArray fixture token at INIT TIME before the run begins.
 * Enforces that the token has at least 180 seconds of remaining validity, matching the
 * bounded budget guard for a standard 150s load-test run (150s run + 30s buffer).
 *
 * Must be called during the k6 init stage (outside default/setup/teardown functions)
 * for every fixture token loaded from a SharedArray, so that stale tokens are caught
 * before the test run starts rather than mid-flight.
 *
 * @param {string} token - JWT string from fixture (e.g. per-user SharedArray entry)
 * @param {object} [cfg=getConfig()] - Config object
 * @returns {object} Validated claims
 */
export function validateFixtureTokenAtInit(token, cfg = getConfig(), fixtureUserId = undefined) {
  let effectiveCfg = cfg;
  let expectedSub = fixtureUserId;
  if (typeof cfg === "string") {
    expectedSub = cfg;
    effectiveCfg = getConfig();
  }
  if (expectedSub && typeof effectiveCfg === "object") {
    effectiveCfg = { ...effectiveCfg, testUserId: expectedSub };
  }
  const claims = validateAuthHubToken(token, effectiveCfg, { minRemainingSeconds: 180 });
  if (expectedSub && claims.sub !== expectedSub) {
    throw new Error(`Token sub claim mismatch. expected=${expectedSub} got=${claims.sub}`);
  }
  return claims;
}

/**
 * Returns headers for authenticated API requests.
 * Uses a pre-issued short-lived token provided via environment variable,
 * or an optional per-user token fixture.
 *
 * When passing a `tokenOverride` from a per-user SharedArray fixture, callers
 * MUST have already called `validateFixtureTokenAtInit(token, cfg)` at init time
 * to enforce the remaining-budget guard. Per-request validation here only checks
 * expiry; it does NOT enforce remaining-budget unless `validateOptions` is provided.
 *
 * @param {string} [tokenOverride] - Optional token override (e.g. from per-user token fixture)
 * @param {object} [validateOptions] - Optional validation options passed to validateAuthHubToken
 *   (e.g. `{ minRemainingSeconds: N }`). Defaults to `{}` for backward compatibility.
 * @returns {object} HTTP headers with Content-Type and X-Alt-Backend-Token
 */
export function getAuthHeaders(tokenOverride, validateOptions = {}) {
  const cfg = getConfig();
  const token = tokenOverride || cfg.apiToken;
  validateAuthHubToken(token, cfg, validateOptions);

  return {
    "Content-Type": "application/json",
    "X-Alt-Backend-Token": token,
  };
}

/** Returns headers for public (unauthenticated) API requests. */
export function getPublicHeaders() {
  return {
    "Content-Type": "application/json",
  };
}
