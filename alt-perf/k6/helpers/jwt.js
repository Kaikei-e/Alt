// k6/helpers/jwt.js - JWT token decoding for K6 load test authentication
//
// All endpoints use X-Alt-Backend-Token (JWT) for authentication.
// This helper validates short-lived scoped tokens issued by AuthHub.
// Compatible with both k6 runtime and Node.js test environments.

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

    let payloadStr;
    if (typeof Buffer !== "undefined") {
      payloadStr = Buffer.from(parts[1], "base64url").toString("utf-8");
    } else {
      let base64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
      while (base64.length % 4) {
        base64 += "=";
      }
      payloadStr = atob(base64);
    }

    return JSON.parse(payloadStr);
  } catch (e) {
    return null;
  }
}
