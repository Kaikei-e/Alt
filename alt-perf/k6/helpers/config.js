// k6/helpers/config.js - Environment variable configuration for K6 load tests
//
// Reads configuration from environment variables with sensible defaults.
// When running in Docker Compose, secrets are injected via docker-entrypoint.sh.

/** @returns {object} K6 test configuration */
export function getConfig() {
  return {
    baseUrl: __ENV.K6_BASE_URL || "http://alt-backend:9000",
    authSecret: __ENV.K6_AUTH_SECRET || "",
    testUserId: __ENV.K6_TEST_USER_ID || "",
    testTenantId: __ENV.K6_TEST_TENANT_ID || "",
    testUserEmail: __ENV.K6_TEST_USER_EMAIL || "",
  };
}
