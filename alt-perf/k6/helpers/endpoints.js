// k6/helpers/endpoints.js - Whitelisted test endpoints for K6 load tests
//
// ETHICAL CONSTRAINT (Multi-layer defense, Layer 3):
// Only endpoints that access LOCAL resources (PostgreSQL, Meilisearch) are listed.
// Endpoints that trigger external API requests are EXCLUDED.
//
// EXCLUDED (external API calls):
//   - /v1/summarize, /v1/articles/*/summarize  (LLM API)
//   - /v1/images/fetch                          (external image fetch)
//   - /v1/augur/*                               (Knowledge Augur / LLM)
//   - /v1/rss-feed-link/register                (external RSS fetch)
//   - /v1/articles/*/archive                    (external archival)
//   - /v1/recap/7days                           (Connect-RPC, migrated)
//
// Adding new endpoints requires code review.

/** Public endpoints (no authentication required) */
export const PUBLIC_ENDPOINTS = [
  { method: "GET", path: "/v1/health", name: "health" },
  { method: "GET", path: "/v1/csrf-token", name: "csrf-token" },
  { method: "GET", path: "/v1/dashboard/overview", name: "dashboard-overview" },
  { method: "GET", path: "/v1/dashboard/metrics", name: "dashboard-metrics" },
];

/** Authenticated endpoints (require shared-secret headers) */
export const AUTH_ENDPOINTS = [
  // Feeds
  { method: "GET", path: "/v1/feeds/fetch/cursor", name: "feeds-cursor", group: "feeds" },
  { method: "GET", path: "/v1/feeds/count/unreads", name: "feeds-unreads", group: "feeds" },
  { method: "GET", path: "/v1/feeds/stats", name: "feeds-stats", group: "stats" },
  { method: "GET", path: "/v1/feeds/stats/detailed", name: "feeds-stats-detailed", group: "stats" },
  { method: "GET", path: "/v1/feeds/stats/trends", name: "feeds-stats-trends", group: "stats" },
  // Search (POST - requires CSRF token)
  { method: "POST", path: "/v1/feeds/search", name: "feeds-search", group: "search", requiresCsrf: true },
  // Articles
  { method: "GET", path: "/v1/articles/by-tag?tag_name=technology", name: "articles-by-tag", group: "articles" },
  // Morning Letter
  { method: "GET", path: "/v1/morning-letter/updates", name: "morning-letter", group: "feeds" },
];

/**
 * Weighted random endpoint selection for load/stress/spike scenarios.
 * Distribution: feeds 40%, stats 20%, search 20%, articles 10%, health 10%
 */
const WEIGHT_MAP = {
  feeds: 40,
  stats: 20,
  search: 20,
  articles: 10,
};

export function pickWeightedEndpoint() {
  const rand = Math.random() * 100;
  let cumulative = 0;

  // Health check (10%)
  if (rand < 10) {
    return PUBLIC_ENDPOINTS[0]; // /v1/health
  }
  cumulative = 10;

  for (const [group, weight] of Object.entries(WEIGHT_MAP)) {
    cumulative += weight;
    if (rand < cumulative) {
      const candidates = AUTH_ENDPOINTS.filter((e) => e.group === group);
      return candidates[Math.floor(Math.random() * candidates.length)];
    }
  }

  // Fallback
  return AUTH_ENDPOINTS[0];
}
