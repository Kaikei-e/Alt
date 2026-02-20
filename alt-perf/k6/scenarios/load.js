// k6/scenarios/load.js - Load test (ramping-vus, 16 min)
//
// Purpose: Simulate normal traffic with gradual VU ramp-up.
// Stages: 0→10 (2m) → hold 10 (5m) → 10→20 (2m) → hold 20 (5m) → 20→0 (2m)
//
// Uses weighted random endpoint selection:
//   feeds 40%, stats 20%, search 20%, articles 10%, health 10%
//
// ETHICAL CONSTRAINT (Layer 2): 10-second cooldown at end of each iteration.

import http from "k6/http";
import { group, sleep } from "k6";
import { getConfig } from "../helpers/config.js";
import { getAuthHeaders, getPublicHeaders } from "../helpers/auth.js";
import { PUBLIC_ENDPOINTS, pickWeightedEndpoint } from "../helpers/endpoints.js";
import { checkResponse, checkHealth, checkAuthResponse } from "../helpers/checks.js";
import { recordResponseTime, recordError } from "../helpers/metrics.js";
import { standardThresholds } from "../config/thresholds.js";
import { handleSummary } from "../helpers/summary.js";

export const options = {
  scenarios: {
    load: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "2m", target: 10 },
        { duration: "5m", target: 10 },
        { duration: "2m", target: 20 },
        { duration: "5m", target: 20 },
        { duration: "2m", target: 0 },
      ],
      gracefulRampDown: "30s",
    },
  },
  thresholds: standardThresholds,
};

export default function () {
  const cfg = getConfig();
  const authHeaders = getAuthHeaders();
  const publicHeaders = getPublicHeaders();

  const ep = pickWeightedEndpoint();

  if (ep.path === "/v1/health") {
    // Public health check
    group("public", () => {
      const res = http.get(`${cfg.baseUrl}${ep.path}`, {
        headers: publicHeaders,
        tags: { name: ep.name },
      });
      checkHealth(res);
    });
  } else if (ep.method === "POST") {
    // POST with CSRF token
    group("authenticated", () => {
      // Fetch CSRF token first
      const csrfRes = http.get(`${cfg.baseUrl}/v1/csrf-token`, {
        headers: publicHeaders,
        tags: { name: "csrf-token" },
      });
      let csrfToken = "";
      if (csrfRes.status === 200) {
        try {
          csrfToken = JSON.parse(csrfRes.body).csrf_token || "";
        } catch (_) {
          // ignore
        }
      }

      const headers = { ...authHeaders };
      if (csrfToken) {
        headers["X-CSRF-Token"] = csrfToken;
      }

      const res = http.post(
        `${cfg.baseUrl}${ep.path}`,
        JSON.stringify({ query: "test", limit: 10 }),
        { headers, tags: { name: ep.name } },
      );
      checkResponse(res, ep.name);
      checkAuthResponse(res, ep.name);
      recordResponseTime(ep.group || "search", res.timings.duration);
      if (res.status >= 400) recordError(res);
    });
  } else {
    // GET authenticated endpoint
    group("authenticated", () => {
      const res = http.get(`${cfg.baseUrl}${ep.path}`, {
        headers: authHeaders,
        tags: { name: ep.name },
      });
      checkResponse(res, ep.name);
      checkAuthResponse(res, ep.name);
      recordResponseTime(ep.group || "feeds", res.timings.duration);
      if (res.status >= 400) recordError(res);
    });
  }

  // Layer 2: 10-second cooldown between iterations
  sleep(10);
}

export { handleSummary };
