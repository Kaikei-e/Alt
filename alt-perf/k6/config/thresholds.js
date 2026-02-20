// k6/config/thresholds.js - Scenario-specific threshold definitions
//
// Aligned with alt-perf/config/thresholds.yaml load test section:
//   p50: 200ms, p95: 500ms, p99: 1000ms, error rate: 1%

/** Standard thresholds (load, soak) */
export const standardThresholds = {
  http_req_duration: ["p(50)<200", "p(95)<500", "p(99)<1000"],
  http_req_failed: ["rate<0.01"],
  checks: ["rate>0.99"],
  successful_checks: ["rate>0.95"],
};

/** Smoke test thresholds (tighter) */
export const smokeThresholds = {
  http_req_duration: ["p(95)<300"],
  http_req_failed: ["rate<0.01"],
  checks: ["rate>0.99"],
  successful_checks: ["rate>0.95"],
};

/** Stress test thresholds (relaxed) */
export const stressThresholds = {
  http_req_duration: ["p(95)<1000"],
  http_req_failed: ["rate<0.05"],
  checks: ["rate>0.95"],
  successful_checks: ["rate>0.90"],
};

/** Soak test thresholds (standard + stability) */
export const soakThresholds = {
  http_req_duration: ["p(50)<200", "p(95)<500"],
  http_req_failed: ["rate<0.01"],
  checks: ["rate>0.99"],
  successful_checks: ["rate>0.95"],
};

/** Spike test thresholds (most relaxed during spike) */
export const spikeThresholds = {
  http_req_duration: ["p(95)<1000"],
  http_req_failed: ["rate<0.05"],
  checks: ["rate>0.95"],
  successful_checks: ["rate>0.90"],
};
