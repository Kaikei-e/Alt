// k6/scenarios/stress.js - Stress test (ramping-vus, 19 min)
//
// Purpose: Push beyond normal capacity to find breaking points.
// Stages: 0→20 (2m) → 20→50 (5m) → 50→100 (2m) → hold 100 (5m) → 100→0 (5m)
//
// Uses weighted random endpoint selection.
//
// ETHICAL CONSTRAINT (Layer 2): 10-second cooldown at end of each iteration.

import http from "k6/http";
import { group, sleep } from "k6";
import { getConfig } from "../helpers/config.js";
import { getAuthHeaders, getPublicHeaders } from "../helpers/auth.js";
import { pickWeightedEndpoint } from "../helpers/endpoints.js";
import { checkResponse, checkHealth, checkAuthResponse } from "../helpers/checks.js";
import { recordResponseTime, recordError } from "../helpers/metrics.js";
import { stressThresholds } from "../config/thresholds.js";
import { handleSummary } from "../helpers/summary.js";

export const options = {
  scenarios: {
    stress: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "2m", target: 20 },
        { duration: "5m", target: 50 },
        { duration: "2m", target: 100 },
        { duration: "5m", target: 100 },
        { duration: "5m", target: 0 },
      ],
      gracefulRampDown: "1m",
    },
  },
  thresholds: stressThresholds,
};

export default function () {
  const cfg = getConfig();
  const authHeaders = getAuthHeaders();
  const publicHeaders = getPublicHeaders();

  const ep = pickWeightedEndpoint();

  if (ep.path === "/v1/health") {
    group("public", () => {
      const res = http.get(`${cfg.baseUrl}${ep.path}`, {
        headers: publicHeaders,
        tags: { name: ep.name },
      });
      checkHealth(res);
    });
  } else if (ep.method === "POST") {
    group("authenticated", () => {
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
