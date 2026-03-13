// k6/helpers/auth.js - Authentication header construction for K6 load tests
//
// Uses X-Alt-Backend-Token (JWT) authentication
// (see alt-backend/app/middleware/auth_middleware.go).

import { getConfig } from "./config.js";
import { generateJWT } from "./jwt.js";

/**
 * Returns headers for authenticated API requests.
 * Issues a short-lived JWT signed with BACKEND_TOKEN_SECRET.
 */
export function getAuthHeaders() {
  const cfg = getConfig();
  const now = Math.floor(Date.now() / 1000);
  const claims = {
    iss: "auth-hub",
    aud: ["alt-backend"],
    sub: cfg.testUserId,
    email: cfg.testUserEmail,
    role: "user",
    sid: "k6-load-test-session",
    iat: now,
    exp: now + 300, // 5 minutes
  };
  const token = generateJWT(cfg.backendTokenSecret, claims);
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
