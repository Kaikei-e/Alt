// k6/scenarios/smoke.js - Smoke test (1 VU, 1 min)
//
// Purpose: Quick sanity check that all whitelisted endpoints are reachable.
// Iterates through every public and authenticated endpoint sequentially.
//
// ETHICAL CONSTRAINT (Layer 2): 10-second cooldown at end of each iteration.

import http from "k6/http";
import { group, sleep } from "k6";
import { getConfig } from "../helpers/config.js";
import { getAuthHeaders, getPublicHeaders } from "../helpers/auth.js";
import { PUBLIC_ENDPOINTS, AUTH_ENDPOINTS } from "../helpers/endpoints.js";
import { checkResponse, checkHealth, checkAuthResponse } from "../helpers/checks.js";
import { recordResponseTime, recordError } from "../helpers/metrics.js";
import { smokeThresholds } from "../config/thresholds.js";
import { handleSummary } from "../helpers/summary.js";

export const options = {
  scenarios: {
    smoke: {
      executor: "constant-vus",
      vus: 1,
      duration: "1m",
    },
  },
  thresholds: smokeThresholds,
};

export default function () {
  const cfg = getConfig();
  const authHeaders = getAuthHeaders();
  const publicHeaders = getPublicHeaders();
  let csrfToken = "";

  // --- Public endpoints ---
  group("public", () => {
    for (const ep of PUBLIC_ENDPOINTS) {
      const res = http.get(`${cfg.baseUrl}${ep.path}`, {
        headers: publicHeaders,
        tags: { name: ep.name },
      });

      if (ep.name === "health") {
        checkHealth(res);
      } else {
        checkResponse(res, ep.name);
      }
      recordResponseTime("dashboard", res.timings.duration);

      // Capture CSRF token for later POST requests
      if (ep.name === "csrf-token" && res.status === 200) {
        try {
          csrfToken = JSON.parse(res.body).csrf_token || "";
        } catch (_) {
          // ignore parse errors
        }
      }
    }
  });

  // --- Authenticated endpoints ---
  group("authenticated", () => {
    for (const ep of AUTH_ENDPOINTS) {
      let res;

      if (ep.method === "POST") {
        const headers = { ...authHeaders };
        if (ep.requiresCsrf && csrfToken) {
          headers["X-CSRF-Token"] = csrfToken;
        }
        res = http.post(
          `${cfg.baseUrl}${ep.path}`,
          JSON.stringify({ query: "test", limit: 5 }),
          { headers, tags: { name: ep.name } },
        );
      } else {
        res = http.get(`${cfg.baseUrl}${ep.path}`, {
          headers: authHeaders,
          tags: { name: ep.name },
        });
      }

      checkResponse(res, ep.name);
      checkAuthResponse(res, ep.name);
      recordResponseTime(ep.group || "feeds", res.timings.duration);

      if (res.status >= 400) {
        recordError(res);
      }
    }
  });

  // Layer 2: 10-second cooldown between iterations
  sleep(10);
}

export { handleSummary };
