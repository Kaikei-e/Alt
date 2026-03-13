// k6/helpers/kratos-auth.js - Kratos browser-flow login for K6 VUs
//
// Authenticates via nginx → Kratos browser login flow.
// Returns the ory_kratos_session cookie value for subsequent requests.

import http from "k6/http";
import { CookieJar } from "k6/http";

const NGINX_BASE = "http://nginx";

/**
 * Extract all cookies from Set-Cookie headers as a "key=value; ..." string.
 */
function extractCookieHeader(res) {
  const setCookies = res.headers["Set-Cookie"];
  if (!setCookies) return "";
  const cookies = Array.isArray(setCookies) ? setCookies : [setCookies];
  return cookies
    .map((c) => c.split(";")[0]) // take "name=value" portion
    .join("; ");
}

/**
 * Perform Kratos browser-flow login and return the session cookie.
 *
 * Flow:
 *   1. GET /ory/self-service/login/browser → flowId + csrf_token + CSRF cookie
 *   2. POST /ory/self-service/login?flow={id} with CSRF cookie → session cookie
 *
 * @param {string} email - User email
 * @param {string} password - User password
 * @returns {string|null} ory_kratos_session cookie value, or null on failure
 */
export function kratosLogin(email, password) {
  // Step 1: Initialize login flow
  const initRes = http.get(`${NGINX_BASE}/ory/self-service/login/browser`, {
    headers: { Accept: "application/json" },
    redirects: 0,
  });

  if (initRes.status !== 200) {
    console.error(
      `kratosLogin: init flow failed status=${initRes.status} email=${email}`,
    );
    return null;
  }

  // Extract cookies from init response (contains csrf_token_* cookie)
  const initCookies = extractCookieHeader(initRes);

  let flowId, csrfToken;
  try {
    const body = JSON.parse(initRes.body);
    flowId = body.id;
    // csrf_token is in ui.nodes
    const csrfNode = body.ui.nodes.find(
      (n) => n.attributes && n.attributes.name === "csrf_token",
    );
    csrfToken = csrfNode ? csrfNode.attributes.value : "";
  } catch (e) {
    console.error(`kratosLogin: parse init response failed: ${e}`);
    return null;
  }

  if (!flowId) {
    console.error("kratosLogin: no flowId in response");
    return null;
  }

  // Step 2: Submit credentials
  // Must include CSRF cookie from init response (Kratos CSRF protection).
  const loginRes = http.post(
    `${NGINX_BASE}/ory/self-service/login?flow=${flowId}`,
    JSON.stringify({
      method: "password",
      identifier: email,
      password: password,
      csrf_token: csrfToken,
    }),
    {
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
        Cookie: initCookies,
      },
      redirects: 0,
    },
  );

  if (loginRes.status !== 200) {
    console.error(
      `kratosLogin: login failed status=${loginRes.status} email=${email} body=${loginRes.body}`,
    );
    return null;
  }

  // Step 3: Extract session cookie from Set-Cookie headers
  // Cookie domain may not match nginx hostname in the load-test environment,
  // so we extract manually from response headers.
  const setCookies = loginRes.headers["Set-Cookie"];
  if (!setCookies) {
    console.error("kratosLogin: no Set-Cookie header in login response");
    return null;
  }

  const cookies = Array.isArray(setCookies) ? setCookies : [setCookies];
  for (const cookie of cookies) {
    const match = cookie.match(/ory_kratos_session=([^;]+)/);
    if (match) {
      return match[1];
    }
  }

  console.error("kratosLogin: ory_kratos_session not found in cookies");
  return null;
}
