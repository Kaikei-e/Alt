// k6/helpers/summary.js - handleSummary for K6 load test report output
//
// Outputs both CLI text summary and JSON report file.

import { textSummary } from "https://jslib.k6.io/k6-summary/0.1.0/index.js";

/**
 * K6 handleSummary hook.
 * Produces CLI text output + JSON file in /app/reports/.
 */
export function handleSummary(data) {
  const timestamp = new Date().toISOString().replace(/[:.]/g, "-");
  const jsonPath = `/app/reports/k6-${timestamp}.json`;

  return {
    stdout: textSummary(data, { indent: "  ", enableColors: true }),
    [jsonPath]: JSON.stringify(data, null, 2),
  };
}
