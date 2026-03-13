// k6/scenarios/sv-connect-composite-10m.js
//
// Composite 10-minute load test for the production SV frontend Connect-RPC path:
// browser -> /sv/api/v2/* -> SvelteKit proxy -> alt-backend Connect-RPC
//
// External HTTP is constrained to the in-repo mock RSS server:
//   - RSS registration: RegisterRSSFeed -> mock-rss
//   - Article extraction: FetchArticleContent -> mock-rss
//
// Workload model:
//   - open model via ramping-arrival-rate, so session starts stay independent
//     from server slowdown
//   - each iteration represents one user session with think time
//
// 10 minute timeline (sessions/sec):
//   0-1m   : base
//   1-3m   : ramp to target
//   3-7m   : hold target
//   7-9m   : ramp to peak
//   9-10m  : hold peak

import http from "k6/http";
import { check, sleep } from "k6";
import { SharedArray } from "k6/data";
import { Counter, Rate, Trend } from "k6/metrics";
import { kratosLogin } from "../helpers/kratos-auth.js";
import { handleSummary } from "../helpers/summary.js";

const users = new SharedArray("load-test-users", function () {
  return JSON.parse(
    open(__ENV.USERS_FILE || "/scripts/data/load-test-users.json"),
  );
});

const APP_BASE = (__ENV.K6_APP_BASE_URL || "http://nginx/sv/api/v2").replace(
  /\/$/,
  "",
);
const MOCK_RSS_BASE = (
  __ENV.MOCK_RSS_BASE_URL || "http://mock-rss-001:8080"
).replace(/\/$/, "");
const BASE_RATE = parseInt(__ENV.SESSION_RATE_BASE || "2", 10);
const TARGET_RATE = parseInt(__ENV.SESSION_RATE_TARGET || "5", 10);
const PEAK_RATE = parseInt(__ENV.SESSION_RATE_PEAK || "8", 10);
const PRE_ALLOCATED_VUS = parseInt(__ENV.PRE_ALLOCATED_VUS || "32", 10);
const MAX_VUS = parseInt(__ENV.MAX_VUS || "128", 10);

const SEARCH_QUERIES = ["tech", "ai", "golang", "svelte", "rust", "news"];

const sessionDuration = new Trend("session_duration", true);
const browseFlowDuration = new Trend("browse_flow_duration", true);
const readFlowDuration = new Trend("read_flow_duration", true);
const discoveryFlowDuration = new Trend("discovery_flow_duration", true);
const manageFlowDuration = new Trend("manage_flow_duration", true);

const unreadCountDuration = new Trend("rpc_unread_count_duration", true);
const unreadFeedsDuration = new Trend("rpc_unread_feeds_duration", true);
const allFeedsDuration = new Trend("rpc_all_feeds_duration", true);
const readFeedsDuration = new Trend("rpc_read_feeds_duration", true);
const favoriteFeedsDuration = new Trend("rpc_favorite_feeds_duration", true);
const statsDuration = new Trend("rpc_stats_duration", true);
const detailedStatsDuration = new Trend("rpc_detailed_stats_duration", true);
const searchDuration = new Trend("rpc_search_duration", true);
const articlesCursorDuration = new Trend("rpc_articles_cursor_duration", true);
const contentDuration = new Trend("rpc_fetch_article_content_duration", true);
const markAsReadDuration = new Trend("rpc_mark_as_read_duration", true);
const listRSSDuration = new Trend("rpc_list_rss_duration", true);
const registerRSSDuration = new Trend("rpc_register_rss_duration", true);
const favoriteDuration = new Trend("rpc_favorite_mutation_duration", true);

const sessionSuccess = new Rate("session_success");
const browseFlowSuccess = new Rate("browse_flow_success");
const readFlowSuccess = new Rate("read_flow_success");
const discoveryFlowSuccess = new Rate("discovery_flow_success");
const manageFlowSuccess = new Rate("manage_flow_success");

const loginErrors = new Counter("login_errors");
const authErrors = new Counter("auth_errors");
const serverErrors = new Counter("server_errors");
const connectErrors = new Counter("connect_errors");
const rateLimitHits = new Counter("rate_limit_hits");

export const options = {
  scenarios: {
    sv_connect_composite_10m: {
      executor: "ramping-arrival-rate",
      startRate: BASE_RATE,
      timeUnit: "1s",
      preAllocatedVUs: PRE_ALLOCATED_VUS,
      maxVUs: MAX_VUS,
      stages: [
        { duration: "1m", target: BASE_RATE },
        { duration: "2m", target: TARGET_RATE },
        { duration: "4m", target: TARGET_RATE },
        { duration: "2m", target: PEAK_RATE },
        { duration: "1m", target: PEAK_RATE },
      ],
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.02"],
    session_success: ["rate>0.97"],
    session_duration: ["p(95)<15000"],
    browse_flow_success: ["rate>0.98"],
    read_flow_success: ["rate>0.95"],
    discovery_flow_success: ["rate>0.97"],
    manage_flow_success: ["rate>0.90"],
    rpc_unread_feeds_duration: ["p(95)<1500"],
    rpc_all_feeds_duration: ["p(95)<1800"],
    rpc_fetch_article_content_duration: ["p(95)<4000"],
    rpc_register_rss_duration: ["p(95)<4000"],
  },
};

const RPC = {
  getUnreadCount: "/alt.feeds.v2.FeedService/GetUnreadCount",
  getUnreadFeeds: "/alt.feeds.v2.FeedService/GetUnreadFeeds",
  getAllFeeds: "/alt.feeds.v2.FeedService/GetAllFeeds",
  getReadFeeds: "/alt.feeds.v2.FeedService/GetReadFeeds",
  getFavoriteFeeds: "/alt.feeds.v2.FeedService/GetFavoriteFeeds",
  getFeedStats: "/alt.feeds.v2.FeedService/GetFeedStats",
  getDetailedFeedStats: "/alt.feeds.v2.FeedService/GetDetailedFeedStats",
  searchFeeds: "/alt.feeds.v2.FeedService/SearchFeeds",
  markAsRead: "/alt.feeds.v2.FeedService/MarkAsRead",
  fetchArticlesCursor: "/alt.articles.v2.ArticleService/FetchArticlesCursor",
  fetchArticleContent: "/alt.articles.v2.ArticleService/FetchArticleContent",
  listRSSFeedLinks: "/alt.rss.v2.RSSService/ListRSSFeedLinks",
  registerRSSFeed: "/alt.rss.v2.RSSService/RegisterRSSFeed",
  registerFavoriteFeed: "/alt.rss.v2.RSSService/RegisterFavoriteFeed",
};

let vuSessionCookie = null;
let vuLastRegisteredFeedUrl = null;
let vuLastRegisteredArticleUrl = null;

function think(minSeconds, maxSeconds) {
  sleep(minSeconds + Math.random() * (maxSeconds - minSeconds));
}

function parseJsonBody(res) {
  if (!res || !res.body) {
    return null;
  }
  try {
    return JSON.parse(res.body);
  } catch (_) {
    return null;
  }
}

function selectUser() {
  if (users.length === 0) {
    throw new Error("USERS_FILE is empty. Run feed-load-test-setup.ts first.");
  }
  const user = users[(__VU - 1) % users.length];
  if (!user.password) {
    throw new Error(
      "USERS_FILE must include passwords. Use /scripts/data/load-test-users.json.",
    );
  }
  return user;
}

function ensureLoggedIn(user) {
  if (vuSessionCookie) {
    return true;
  }
  const session = kratosLogin(user.email, user.password);
  if (!session) {
    loginErrors.add(1);
    return false;
  }
  vuSessionCookie = session;
  return true;
}

function recordResponse(res, trendMetric, tagName, expectedStatuses = [200]) {
  if (!res) {
    connectErrors.add(1);
    return false;
  }

  if (trendMetric && res.timings) {
    trendMetric.add(res.timings.duration);
  }

  if (res.status === 429) {
    rateLimitHits.add(1);
  } else if (res.status === 401 || res.status === 403) {
    authErrors.add(1);
  } else if (res.status >= 500) {
    serverErrors.add(1);
  } else if (!expectedStatuses.includes(res.status)) {
    connectErrors.add(1);
  }

  const label = tagName || (res.request && res.request.tags && res.request.tags.name) || "unknown";
  return check(res, {
    [`${label}: expected status`]: (r) =>
      expectedStatuses.includes(r.status),
  });
}

function connectPost(path, body, name, trendMetric, expectedStatuses = [200]) {
  const res = http.post(`${APP_BASE}${path}`, JSON.stringify(body), {
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
      Cookie: `ory_kratos_session=${vuSessionCookie}`,
    },
    tags: { name },
  });

  const ok = recordResponse(res, trendMetric, name, expectedStatuses);
  return { ok, res, body: parseJsonBody(res) };
}

function mockFeedKey() {
  return `vu-${String(__VU).padStart(4, "0")}-iter-${String(__ITER).padStart(6, "0")}`;
}

function buildMockFeedUrl(feedKey) {
  return `${MOCK_RSS_BASE}/feeds/${feedKey}/rss.xml`;
}

function buildMockArticleUrl(feedKey, itemNumber = 1) {
  return `${MOCK_RSS_BASE}/articles/feed-${feedKey}/item-${itemNumber}`;
}

function pickSearchQuery() {
  return SEARCH_QUERIES[Math.floor(Math.random() * SEARCH_QUERIES.length)];
}

function flowBrowse() {
  const started = Date.now();
  let ok = true;

  const unreadCount = connectPost(
    RPC.getUnreadCount,
    {},
    "sv-unread-count",
    unreadCountDuration,
  );
  ok = unreadCount.ok && ok;
  think(0.2, 0.6);

  const unreadFeeds = connectPost(
    RPC.getUnreadFeeds,
    { limit: 20 },
    "sv-unread-feeds",
    unreadFeedsDuration,
  );
  ok = unreadFeeds.ok && ok;
  think(0.3, 0.8);

  const unreadCursor = unreadFeeds.body && unreadFeeds.body.nextCursor
    ? unreadFeeds.body.nextCursor
    : null;

  const allFeeds = connectPost(
    RPC.getAllFeeds,
    unreadCursor ? { limit: 20, cursor: unreadCursor } : { limit: 20 },
    "sv-all-feeds",
    allFeedsDuration,
  );
  ok = allFeeds.ok && ok;

  if (allFeeds.body && allFeeds.body.nextCursor) {
    think(0.3, 0.7);
    const nextPage = connectPost(
      RPC.getAllFeeds,
      { limit: 20, cursor: allFeeds.body.nextCursor },
      "sv-all-feeds-next",
      allFeedsDuration,
    );
    ok = nextPage.ok && ok;
  }

  browseFlowDuration.add(Date.now() - started);
  browseFlowSuccess.add(ok);
  return ok;
}

function flowRead() {
  const started = Date.now();
  let ok = true;

  const unreadFeeds = connectPost(
    RPC.getUnreadFeeds,
    { limit: 1, view: "swipe" },
    "sv-read-unread-feed",
    unreadFeedsDuration,
  );
  ok = unreadFeeds.ok && ok;
  think(0.2, 0.5);

  const articles = connectPost(
    RPC.fetchArticlesCursor,
    { limit: 5 },
    "sv-articles-cursor",
    articlesCursorDuration,
  );
  ok = articles.ok && ok;
  think(0.4, 0.9);

  const mockArticleUrl = vuLastRegisteredArticleUrl || buildMockArticleUrl(mockFeedKey(), 1);
  const fetchContent = connectPost(
    RPC.fetchArticleContent,
    { url: mockArticleUrl },
    "sv-fetch-article-content",
    contentDuration,
  );
  ok = fetchContent.ok && ok;

  let articleUrlToMark = vuLastRegisteredArticleUrl;
  if (
    !articleUrlToMark &&
    unreadFeeds.body &&
    Array.isArray(unreadFeeds.body.data) &&
    unreadFeeds.body.data.length > 0
  ) {
    articleUrlToMark = unreadFeeds.body.data[0].link || null;
  }

  if (articleUrlToMark) {
    think(0.2, 0.5);
    const markAsRead = connectPost(
      RPC.markAsRead,
      { articleUrl: articleUrlToMark },
      "sv-mark-as-read",
      markAsReadDuration,
      [200, 404],
    );
    ok = markAsRead.ok && ok;
  }

  think(0.2, 0.4);
  const unreadCount = connectPost(
    RPC.getUnreadCount,
    {},
    "sv-read-unread-count",
    unreadCountDuration,
  );
  ok = unreadCount.ok && ok;

  readFlowDuration.add(Date.now() - started);
  readFlowSuccess.add(ok);
  return ok;
}

function flowDiscovery() {
  const started = Date.now();
  let ok = true;

  const feedStats = connectPost(
    RPC.getFeedStats,
    {},
    "sv-feed-stats",
    statsDuration,
  );
  ok = feedStats.ok && ok;
  think(0.1, 0.3);

  const detailedStats = connectPost(
    RPC.getDetailedFeedStats,
    {},
    "sv-detailed-feed-stats",
    detailedStatsDuration,
  );
  ok = detailedStats.ok && ok;
  think(0.2, 0.6);

  const search = connectPost(
    RPC.searchFeeds,
    { query: pickSearchQuery(), limit: 20 },
    "sv-search-feeds",
    searchDuration,
  );
  ok = search.ok && ok;

  think(0.2, 0.5);
  const readFeeds = connectPost(
    RPC.getReadFeeds,
    { limit: 20 },
    "sv-read-feeds",
    readFeedsDuration,
  );
  ok = readFeeds.ok && ok;

  think(0.2, 0.4);
  const favoriteFeeds = connectPost(
    RPC.getFavoriteFeeds,
    { limit: 20 },
    "sv-favorite-feeds",
    favoriteFeedsDuration,
  );
  ok = favoriteFeeds.ok && ok;

  discoveryFlowDuration.add(Date.now() - started);
  discoveryFlowSuccess.add(ok);
  return ok;
}

function flowManageFeeds() {
  const started = Date.now();
  let ok = true;

  const listBefore = connectPost(
    RPC.listRSSFeedLinks,
    {},
    "sv-list-rss-before",
    listRSSDuration,
  );
  ok = listBefore.ok && ok;
  think(0.2, 0.5);

  const feedKey = mockFeedKey();
  const feedUrl = buildMockFeedUrl(feedKey);
  const articleUrl = buildMockArticleUrl(feedKey, 1);

  const registerFeed = connectPost(
    RPC.registerRSSFeed,
    { url: feedUrl },
    "sv-register-rss",
    registerRSSDuration,
  );
  ok = registerFeed.ok && ok;

  if (registerFeed.ok) {
    vuLastRegisteredFeedUrl = feedUrl;
    vuLastRegisteredArticleUrl = articleUrl;
  }

  think(0.4, 0.9);
  const registerFavorite = connectPost(
    RPC.registerFavoriteFeed,
    { url: articleUrl },
    "sv-register-favorite",
    favoriteDuration,
  );
  ok = registerFavorite.ok && ok;

  think(0.2, 0.5);
  const listAfter = connectPost(
    RPC.listRSSFeedLinks,
    {},
    "sv-list-rss-after",
    listRSSDuration,
  );
  ok = listAfter.ok && ok;

  manageFlowDuration.add(Date.now() - started);
  manageFlowSuccess.add(ok);
  return ok;
}

export default function () {
  const user = selectUser();
  if (!ensureLoggedIn(user)) {
    sessionSuccess.add(false);
    return;
  }

  const sessionStarted = Date.now();
  const roll = Math.random() * 100;
  let ok;

  if (roll < 45) {
    ok = flowBrowse();
  } else if (roll < 70) {
    ok = flowRead();
  } else if (roll < 90) {
    ok = flowDiscovery();
  } else {
    ok = flowManageFeeds();
  }

  sessionDuration.add(Date.now() - sessionStarted);
  sessionSuccess.add(ok);
}

export { handleSummary };
