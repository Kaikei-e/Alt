// k6/helpers/auth.js - Authentication header construction for K6 load tests
//
// Uses X-Alt-Shared-Secret fallback authentication path
// (see alt-backend/app/middleware/auth_middleware.go).

import { getConfig } from "./config.js";

/**
 * Returns headers for authenticated API requests.
 * Uses shared-secret fallback auth: X-Alt-Shared-Secret + user identity headers.
 */
export function getAuthHeaders() {
  const cfg = getConfig();
  return {
    "Content-Type": "application/json",
    "X-Alt-Shared-Secret": cfg.authSecret,
    "X-Alt-User-Id": cfg.testUserId,
    "X-Alt-Tenant-Id": cfg.testTenantId,
    "X-Alt-User-Email": cfg.testUserEmail,
  };
}

/** Returns headers for public (unauthenticated) API requests. */
export function getPublicHeaders() {
  return {
    "Content-Type": "application/json",
  };
}
