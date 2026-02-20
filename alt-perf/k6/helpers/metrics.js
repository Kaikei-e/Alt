// k6/helpers/metrics.js - Custom K6 metrics for load tests

import { Counter, Rate, Trend } from "k6/metrics";

// Group-specific response time trends
export const feedsResponseTime = new Trend("feeds_response_time", true);
export const searchResponseTime = new Trend("search_response_time", true);
export const dashboardResponseTime = new Trend("dashboard_response_time", true);
export const articlesResponseTime = new Trend("articles_response_time", true);

// Error counters
export const authErrors = new Counter("auth_errors");
export const serverErrors = new Counter("server_errors");

// Success rate
export const successfulChecks = new Rate("successful_checks");

/** Record response time to the appropriate group-specific trend metric */
export function recordResponseTime(group, duration) {
  switch (group) {
    case "feeds":
      feedsResponseTime.add(duration);
      break;
    case "search":
      searchResponseTime.add(duration);
      break;
    case "dashboard":
      dashboardResponseTime.add(duration);
      break;
    case "articles":
      articlesResponseTime.add(duration);
      break;
  }
}

/** Record error by type */
export function recordError(res) {
  if (res.status === 401 || res.status === 403) {
    authErrors.add(1);
  }
  if (res.status >= 500) {
    serverErrors.add(1);
  }
}
