// k6/scenarios/soak.js - Soak test (constant 10 VUs, 30 min)
//
// Purpose: Detect memory leaks, connection pool exhaustion, and gradual
// performance degradation under sustained moderate load.
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
import { soakThresholds } from "../config/thresholds.js";
import { handleSummary } from "../helpers/summary.js";

export const options = {
  scenarios: {
    soak: {
      executor: "constant-vus",
      vus: 10,
      duration: "30m",
    },
  },
  thresholds: soakThresholds,
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
