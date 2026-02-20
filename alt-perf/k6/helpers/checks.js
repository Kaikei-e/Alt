// k6/helpers/checks.js - Response validation functions for K6 load tests

import { check } from "k6";
import { successfulChecks } from "./metrics.js";

/** Check standard API response: status 200 and response time < 500ms */
export function checkResponse(res, name) {
  const passed = check(res, {
    [`${name}: status 200`]: (r) => r.status === 200,
    [`${name}: response time < 500ms`]: (r) => r.timings.duration < 500,
  });
  successfulChecks.add(passed ? 1 : 0);
  return passed;
}

/** Check health endpoint: status 200 and body contains "healthy" */
export function checkHealth(res) {
  const passed = check(res, {
    "health: status 200": (r) => r.status === 200,
    'health: body contains "healthy"': (r) =>
      r.body && r.body.includes("healthy"),
  });
  successfulChecks.add(passed ? 1 : 0);
  return passed;
}

/** Check authenticated response: not 401/403 (auth rejection) */
export function checkAuthResponse(res, name) {
  const passed = check(res, {
    [`${name}: not 401`]: (r) => r.status !== 401,
    [`${name}: not 403`]: (r) => r.status !== 403,
  });
  successfulChecks.add(passed ? 1 : 0);
  return passed;
}
