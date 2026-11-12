// k6/helpers/jwt.js - JWT token decoding for K6 load test authentication
//
// All endpoints use X-Alt-Backend-Token (JWT) for authentication.
// This helper validates short-lived scoped tokens issued by AuthHub.
// Compatible with both k6 runtime and Node.js test environments.

function decodePart(part) {
  let str;
  if (typeof Buffer !== "undefined") {
    str = Buffer.from(part, "base64url").toString("utf-8");
  } else {
    let base64 = part.replace(/-/g, "+").replace(/_/g, "/");
    while (base64.length % 4) {
      base64 += "=";
    }
    str = atob(base64);
  }
  return JSON.parse(str);
}

/**
 * Decode a JWT header without verifying the cryptographic signature.
 *
 * @param {string} token - JWT string
 * @returns {object|null} Decoded header or null if invalid
 */
export function decodeJWTHeader(token) {
  try {
    if (typeof token !== "string") return null;
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    return decodePart(parts[0]);
  } catch {
    return null;
  }
}

/**
 * Decode a JWT token without verifying the cryptographic signature.
 *
 * @param {string} token - JWT string
 * @returns {object|null} Decoded claims or null if invalid
 */
export function decodeJWT(token) {
  try {
    if (typeof token !== "string") return null;
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    return decodePart(parts[1]);
  } catch {
    return null;
  }
}
