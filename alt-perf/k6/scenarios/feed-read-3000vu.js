// k6/scenarios/feed-read-3000vu.js - Feed read load test (3000 VU, 30 min)
//
// Purpose: Simulate 3000 concurrent users reading feeds via Connect-RPC (v2).
// Executor: ramping-vus (closed model)
// Stages: 0→300 (2m) → 300→1000 (3m) → 1000→2000 (3m) → 2000→3000 (4m)
//         → hold 3000 (15m) → 3000→0 (3m)
//
// Behavior distribution:
//   55% browse:    feed-list → feed-items
//   20% paginate:  feed-list → feed-items → feed-items (cursor)
//   15% deep-read: feed-list → feed-items → item-detail (ArticlesCursor)
//   10% glance:    feed-list only
//
// All endpoints are Connect-RPC v2 (POST + JSON body), DB-read only.
// FetchArticleContent is NOT used (it fetches external URLs → SSRF issues).
// Instead, FetchArticlesCursor is used for item-detail (pure DB read).
//
// Auth: JWT via X-Alt-Backend-Token (Connect-RPC auth interceptor).
// Tokens are HMAC-SHA256 signed with BACKEND_TOKEN_SECRET.
// User IDs must be valid UUIDs (auth interceptor calls uuid.Parse()).
//
// Prerequisites:
//   - Run via ./alt-perf/scripts/run-feed-read-load-test.sh
//     (sets DoS protection overrides, scales resources, targets port 9101)
//   - K6_BACKEND_TOKEN_SECRET injected via Docker secret (docker-entrypoint.sh)

import http from "k6/http";
import { check, sleep } from "k6";
import { SharedArray } from "k6/data";
import { Counter, Trend } from "k6/metrics";
import { getConfig } from "../helpers/config.js";
import { generateJWT } from "../helpers/jwt.js";
import { handleSummary } from "../helpers/summary.js";

// ---------------------------------------------------------------------------
// Data (SharedArray for memory efficiency across 3000 VUs)
// ---------------------------------------------------------------------------

const users = new SharedArray("users", function () {
  return JSON.parse(
    open(__ENV.USERS_FILE || "/scripts/data/users.sample.json"),
  );
});

// ---------------------------------------------------------------------------
// Custom Metrics
// ---------------------------------------------------------------------------

const feedListDuration = new Trend("feed_list_duration", true);
const feedItemsDuration = new Trend("feed_items_duration", true);
const itemDetailDuration = new Trend("item_detail_duration", true);
const rateLimitHits = new Counter("rate_limit_hits");
const authErrors = new Counter("auth_errors");
const serverErrors = new Counter("server_errors");

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

export const options = {
  scenarios: {
    feed_read: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "2m", target: 300 },
        { duration: "3m", target: 1000 },
        { duration: "3m", target: 2000 },
        { duration: "4m", target: 3000 },
        { duration: "15m", target: 3000 },
        { duration: "3m", target: 0 },
      ],
      gracefulRampDown: "30s",
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<1500", "p(99)<3000"],
    feed_list_duration: ["p(95)<800"],
    feed_items_duration: ["p(95)<1200"],
    item_detail_duration: ["p(95)<1000"],
  },
  // Discard response bodies to save memory at 3000 VUs
  discardResponseBodies: true,
};

// ---------------------------------------------------------------------------
// Connect-RPC Endpoints (all DB-read only, no external fetches)
// ---------------------------------------------------------------------------

const RPC = {
  getUnreadFeeds: "/alt.feeds.v2.FeedService/GetUnreadFeeds",
  getAllFeeds: "/alt.feeds.v2.FeedService/GetAllFeeds",
  getUnreadCount: "/alt.feeds.v2.FeedService/GetUnreadCount",
  fetchArticlesCursor: "/alt.articles.v2.ArticleService/FetchArticlesCursor",
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Random sleep between 0.8s and 3.5s (think time) */
function thinkTime() {
  sleep(0.8 + Math.random() * 2.7);
}

/** Build JWT auth headers for Connect-RPC (port 9101) */
function buildAuthHeaders(user) {
  const cfg = getConfig();
  const now = Math.floor(Date.now() / 1000);
  const token = generateJWT(cfg.backendTokenSecret, {
    sub: user.user_id,
    email: user.email,
    role: "user",
    sid: `loadtest-${user.user_id}`,
    iss: "auth-hub",
    aud: ["alt-backend"],
    iat: now,
    exp: now + 300, // 5 min
  });
  return {
    "Content-Type": "application/json",
    "X-Alt-Backend-Token": token,
  };
}

/** Send a Connect-RPC POST request */
function connectPost(baseUrl, rpcPath, body, nameTag, headers) {
  return http.post(`${baseUrl}${rpcPath}`, JSON.stringify(body), {
    headers: headers,
    tags: { name: nameTag },
  });
}

/** Validate response and record metrics */
function checkAndRecord(res, nameTag, trendMetric) {
  if (trendMetric) {
    trendMetric.add(res.timings.duration);
  }

  // 429: rate limited (separate counter, not counted as failure)
  if (res.status === 429) {
    rateLimitHits.add(1);
    return;
  }

  if (res.status === 401 || res.status === 403) {
    authErrors.add(1);
  }

  if (res.status >= 500) {
    serverErrors.add(1);
  }

  check(res, {
    [`${nameTag}: status 200`]: (r) => r.status === 200,
  });
}

// ---------------------------------------------------------------------------
// Scenario Flows
// ---------------------------------------------------------------------------

/** Flow: feed-list only (glance) - 10% */
function flowGlance(baseUrl, headers) {
  const res = connectPost(
    baseUrl,
    RPC.getUnreadFeeds,
    { limit: 20 },
    "feed-list",
    headers,
  );
  checkAndRecord(res, "feed-list", feedListDuration);
}

/** Flow: feed-list → feed-items (browse) - 55% */
function flowBrowse(baseUrl, headers) {
  const listRes = connectPost(
    baseUrl,
    RPC.getUnreadFeeds,
    { limit: 20 },
    "feed-list",
    headers,
  );
  checkAndRecord(listRes, "feed-list", feedListDuration);

  thinkTime();

  const res = connectPost(
    baseUrl,
    RPC.getAllFeeds,
    { limit: 20 },
    "feed-items",
    headers,
  );
  checkAndRecord(res, "feed-items", feedItemsDuration);
}

/** Flow: feed-list → feed-items → next page (paginate) - 20% */
function flowPaginate(baseUrl, headers) {
  const listRes = connectPost(
    baseUrl,
    RPC.getUnreadFeeds,
    { limit: 20 },
    "feed-list",
    headers,
  );
  checkAndRecord(listRes, "feed-list", feedListDuration);

  thinkTime();

  const itemsRes = connectPost(
    baseUrl,
    RPC.getAllFeeds,
    { limit: 20 },
    "feed-items",
    headers,
  );
  checkAndRecord(itemsRes, "feed-items", feedItemsDuration);

  thinkTime();

  // Next page: synthetic RFC3339 cursor (discardResponseBodies prevents reading real cursors)
  const syntheticCursor = new Date(
    Date.now() - Math.floor(Math.random() * 86400000),
  ).toISOString();
  const nextRes = connectPost(
    baseUrl,
    RPC.getAllFeeds,
    { limit: 20, cursor: syntheticCursor },
    "feed-items",
    headers,
  );
  checkAndRecord(nextRes, "feed-items", feedItemsDuration);
}

/** Flow: feed-list → feed-items → item-detail (deep-read) - 15% */
function flowDeepRead(baseUrl, headers) {
  const listRes = connectPost(
    baseUrl,
    RPC.getUnreadFeeds,
    { limit: 20 },
    "feed-list",
    headers,
  );
  checkAndRecord(listRes, "feed-list", feedListDuration);

  thinkTime();

  const itemsRes = connectPost(
    baseUrl,
    RPC.getAllFeeds,
    { limit: 20 },
    "feed-items",
    headers,
  );
  checkAndRecord(itemsRes, "feed-items", feedItemsDuration);

  thinkTime();

  // Article detail: FetchArticlesCursor (DB-only, no external fetch)
  // Uses a synthetic cursor to simulate browsing different article pages
  const syntheticCursor = new Date(
    Date.now() - Math.floor(Math.random() * 604800000), // random within last 7 days
  ).toISOString();
  const detailRes = connectPost(
    baseUrl,
    RPC.fetchArticlesCursor,
    { limit: 5, cursor: syntheticCursor },
    "item-detail",
    headers,
  );
  checkAndRecord(detailRes, "item-detail", itemDetailDuration);
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

export default function () {
  const cfg = getConfig();
  const user = users[(__VU - 1) % users.length];
  const headers = buildAuthHeaders(user);

  // Select behavior based on distribution
  const roll = Math.random() * 100;

  if (roll < 10) {
    flowGlance(cfg.baseUrl, headers);
  } else if (roll < 65) {
    flowBrowse(cfg.baseUrl, headers);
  } else if (roll < 85) {
    flowPaginate(cfg.baseUrl, headers);
  } else {
    flowDeepRead(cfg.baseUrl, headers);
  }

  // Think time at end of iteration
  thinkTime();
}

export { handleSummary };
