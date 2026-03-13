// k6/scenarios/feed-registration.js - Feed registration load test
//
// Each VU authenticates via Kratos, then registers FEED_COUNT feeds
// spread over DURATION seconds with random pacing.
// Each VU simulates a unique client IP via X-Forwarded-For.
//
// Usage:
//   docker compose -f compose/compose.yaml -f compose/load-test.yaml -f compose/perf.yaml -p alt \
//     run --rm -e FEED_COUNT=10 -e DURATION=60 k6 run /scripts/scenarios/feed-registration.js

import http from "k6/http";
import { check, sleep } from "k6";
import { SharedArray } from "k6/data";
import { Counter, Trend } from "k6/metrics";
import { kratosLogin } from "../helpers/kratos-auth.js";
import { handleSummary } from "../helpers/summary.js";

// --- Custom metrics ---
const registerDuration = new Trend("feed_register_duration", true);
const registerErrors = new Counter("feed_register_errors");
const loginErrors = new Counter("feed_login_errors");
const retryCount = new Counter("feed_retry_count");
const serverErrors = new Counter("feed_server_errors");
const rateLimitErrors = new Counter("feed_rate_limit_errors");

// --- Load test users from JSON file ---
const users = new SharedArray("users", function () {
  return JSON.parse(open("/scripts/data/load-test-users.json"));
});

// --- Configuration (overridable via env) ---
const FEED_COUNT = parseInt(__ENV.FEED_COUNT || "100", 10);
const VU_COUNT = parseInt(__ENV.VU_COUNT || String(users.length), 10);
const DURATION = parseInt(__ENV.DURATION || "60", 10);
const ALIAS_COUNT = parseInt(__ENV.ALIAS_COUNT || String(users.length), 10);
const P95_LIMIT = parseInt(__ENV.P95_LIMIT || "15000", 10);
const ERROR_LIMIT = parseInt(__ENV.ERROR_LIMIT || "10000", 10);
const RAMP_UP = parseInt(__ENV.RAMP_UP || "30", 10);
const avgInterval = DURATION / FEED_COUNT;
const NGINX_BASE = "http://nginx";
const BACKEND_BASE = `${NGINX_BASE}/api/backend`;

// --- Options ---
// ramping-vus spreads VU startup over RAMP_UP seconds, avoiding the
// thundering-herd of cache misses that per-vu-iterations caused.
export const options = {
  scenarios: {
    feed_registration: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: `${RAMP_UP}s`, target: VU_COUNT }, // gradual ramp-up
        { duration: `${DURATION}s`, target: VU_COUNT }, // steady state
      ],
      gracefulRampDown: "30s",
    },
  },
  thresholds: {
    feed_register_duration: [`p(95)<${P95_LIMIT}`],
    feed_register_errors: [`count<${ERROR_LIMIT}`],
    http_req_failed: ["rate<0.20"],
  },
};

// --- Retry wrapper with improved backoff ---
function withRetry(fn, maxRetries = 2) {
  for (let i = 0; i <= maxRetries; i++) {
    const res = fn();
    if (!res) return res;

    // Success or client error (non-429) → return immediately
    if (res.status < 500 && res.status !== 429) return res;

    // 429: explicit rate limit → don't retry, respect Retry-After
    if (res.status === 429) {
      rateLimitErrors.add(1);
      const retryAfter = parseInt(res.headers["Retry-After"] || "5", 10);
      sleep(retryAfter + Math.random());
      return res;
    }

    // 500 with short body: likely nginx auth_request 429→500 conversion
    // nginx returns a short HTML error page (<200 bytes) vs backend JSON (>200 bytes)
    const bodyLen = res.body ? res.body.length : 0;
    if (res.status === 500 && bodyLen < 200) {
      rateLimitErrors.add(1);
      sleep(2 + Math.random() * 2); // 2-4s cooldown
      return res; // don't retry — it's rate limiting, not a transient error
    }

    // Real server error → exponential backoff with full jitter
    serverErrors.add(1);
    if (i === maxRetries) return res;
    retryCount.add(1);
    const baseDelay = 2 * Math.pow(2, i); // 2s, 4s
    sleep(baseDelay * Math.random());
  }
}

// VU-local storage (SharedArray objects are frozen)
let vuSession = null;
let vuCsrfToken = null; // CSRF token cached per VU (valid 1hr, test runs << 1hr)
let vuFeedCount = 0; // tracks completed feed registrations per VU

export default function () {
  // ramping-vus doesn't cap iterations per VU; stop after FEED_COUNT feeds
  if (vuFeedCount >= FEED_COUNT) {
    sleep(1);
    return;
  }

  const user = users[(__VU - 1) % users.length];
  if (!user) {
    console.error(`No user for VU ${__VU}`);
    return;
  }

  // Simulate unique client IP per VU for DoS protection middleware
  const vuIP = `10.200.${Math.floor((__VU - 1) / 256)}.${(__VU - 1) % 256 + 1}`;

  // Authenticate on first iteration with small jitter
  if (__ITER === 0) {
    // ramping-vus already spreads VU startup; add small jitter (0-2s) to
    // avoid exact-same-second collisions among VUs starting in the same batch
    sleep(Math.random() * 2);
    const sessionCookie = kratosLogin(user.email, user.password);
    if (!sessionCookie) {
      loginErrors.add(1);
      console.error(`VU ${__VU}: login failed for ${user.email}`);
      return;
    }
    vuSession = sessionCookie;

    // Prefetch CSRF token (valid 1hr, reuse across all iterations)
    const csrfRes = withRetry(() =>
      http.get(`${BACKEND_BASE}/v1/csrf-token`, {
        headers: {
          Cookie: `ory_kratos_session=${vuSession}`,
          "X-Forwarded-For": vuIP,
        },
      }),
    );
    if (csrfRes && csrfRes.status === 200) {
      try {
        vuCsrfToken = JSON.parse(csrfRes.body).csrf_token || "";
      } catch (_) {
        // ignore
      }
    }
  }

  if (!vuSession) {
    // Login failed on iter 0, skip remaining iterations
    registerErrors.add(1);
    return;
  }

  // Feed ID: zero-padded 001..100 (based on vuFeedCount, not __ITER)
  const feedId = String(vuFeedCount + 1).padStart(3, "0");
  // Each VU maps to a DNS alias bucket (10k VUs share 1k aliases)
  const aliasBucket = ((__VU - 1) % ALIAS_COUNT) + 1;
  const vuId = String(aliasBucket).padStart(String(ALIAS_COUNT).length, "0");
  const feedUrl = `http://mock-rss-${vuId}:8080/feeds/${feedId}/rss.xml`;

  // Use cached CSRF token; fallback fetch only if cache is empty
  let csrfToken = vuCsrfToken;
  if (!csrfToken) {
    const csrfRes = withRetry(() =>
      http.get(`${BACKEND_BASE}/v1/csrf-token`, {
        headers: {
          Cookie: `ory_kratos_session=${vuSession}`,
          "X-Forwarded-For": vuIP,
        },
      }),
    );
    if (csrfRes && csrfRes.status === 200) {
      try {
        csrfToken = JSON.parse(csrfRes.body).csrf_token || "";
        vuCsrfToken = csrfToken;
      } catch (_) {
        // ignore
      }
    }
  }

  // Register feed (with retry for 500s)
  const res = withRetry(() =>
    http.post(
      `${BACKEND_BASE}/v1/rss-feed-link/register`,
      JSON.stringify({ url: feedUrl }),
      {
        headers: {
          "Content-Type": "application/json",
          Cookie: `ory_kratos_session=${vuSession}`,
          "X-CSRF-Token": csrfToken,
          "X-Forwarded-For": vuIP,
        },
        tags: { name: "feed-register" },
      },
    ),
  );

  registerDuration.add(res.timings.duration);

  const ok = check(res, {
    "status is 2xx": (r) => r.status >= 200 && r.status < 300,
    "status is not 5xx": (r) => r.status < 500,
  });

  if (!ok) {
    registerErrors.add(1);
    if (vuFeedCount < 3) {
      console.warn(
        `VU ${__VU} feed ${vuFeedCount}: status=${res.status} body=${res.body}`,
      );
    }
  }

  vuFeedCount++;

  // Pace requests over DURATION: avg interval with +-50% jitter
  sleep(avgInterval * (0.5 + Math.random()));
}

export { handleSummary };
