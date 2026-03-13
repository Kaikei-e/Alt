// k6/helpers/jwt.js - JWT token generation for K6 load test authentication
//
// All endpoints use X-Alt-Backend-Token (JWT) for authentication.
// This helper generates HMAC-SHA256 JWTs using k6's built-in
// crypto and encoding modules.

import crypto from "k6/crypto";
import encoding from "k6/encoding";

/**
 * Base64url-encode a string (no padding, URL-safe alphabet).
 * @param {string} input
 * @returns {string}
 */
function base64UrlEncode(input) {
  return encoding.b64encode(input, "rawurl");
}

/**
 * Generate a signed JWT (HS256).
 *
 * @param {string} secret - HMAC secret (BACKEND_TOKEN_SECRET)
 * @param {object} claims - JWT payload claims
 * @returns {string} Signed JWT string
 */
export function generateJWT(secret, claims) {
  const header = '{"alg":"HS256","typ":"JWT"}';
  const encodedHeader = base64UrlEncode(header);
  const encodedPayload = base64UrlEncode(JSON.stringify(claims));
  const signingInput = `${encodedHeader}.${encodedPayload}`;
  const signature = crypto.hmac("sha256", secret, signingInput, "base64rawurl");
  return `${signingInput}.${signature}`;
}
