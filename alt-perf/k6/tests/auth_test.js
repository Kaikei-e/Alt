// alt-perf/k6/tests/auth_test.js
// Unit and functional verification of AuthHub JWT validation for K6 load testing.

import assert from "node:assert";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Helper to base64url encode JSON
function b64url(obj) {
  return Buffer.from(JSON.stringify(obj))
    .toString("base64")
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
}

function makeToken(payload, header = { alg: "HS256", typ: "JWT" }, sig = "dummysig") {
  return `${b64url(header)}.${b64url(payload)}.${sig}`;
}

async function run() {
  globalThis.__ENV = globalThis.__ENV || {};
  console.log("Starting AuthHub JWT and k6 helper verification...");

  const { getAuthHeaders, validateAuthHubToken, validateCohortTokenAtInit } = await import("../helpers/auth.js");
  const { decodeJWT } = await import("../helpers/jwt.js");

  const validUserId = "11111111-1111-4111-a111-111111111111";
  const validTenantId = "22222222-2222-4222-a222-222222222222";
  const now = Math.floor(Date.now() / 1000);

  const baseClaims = {
    sub: validUserId,
    tenant_id: validTenantId,
    email: "test@example.com",
    role: "user",
    sid: "session-123",
    iss: "auth-hub",
    aud: ["alt-backend"],
    iat: now,
    exp: now + 300, // 5 min TTL
  };

  const validToken = makeToken(baseClaims);

  const validConfig = {
    apiToken: validToken,
    testUserId: validUserId,
    testTenantId: validTenantId,
  };

  // 1. Valid token passes validation and returns headers
  {
    const claims = validateAuthHubToken(validToken, validConfig);
    assert.strictEqual(claims.sub, validUserId);
    assert.strictEqual(claims.tenant_id, validTenantId);
    assert.strictEqual(claims.iss, "auth-hub");
    console.log("✓ Valid token passes validation");
  }

  // 2. Token without invented scope claim passes (production AuthHub doesn't emit scope)
  {
    assert.strictEqual(baseClaims.scope, undefined);
    const claims = validateAuthHubToken(validToken, validConfig);
    assert.strictEqual(claims.sub, validUserId);
    console.log("✓ Token without invented scope claim passes");
  }

  // 3. Negative: Missing token throws
  {
    assert.throws(() => {
      validateAuthHubToken("", validConfig);
    }, /Missing.*token/i);
    console.log("✓ Missing token rejected");
  }

  // 4. Negative: Malformed token format throws
  {
    assert.throws(() => {
      validateAuthHubToken("not-a-jwt", validConfig);
    }, /Invalid JWT token format/i);

    assert.throws(() => {
      validateAuthHubToken("a.b", validConfig);
    }, /Invalid JWT token format/i);
    console.log("✓ Malformed token format rejected");
  }

  // 5. Negative: Oversized token throws
  {
    const hugePadding = "x".repeat(5000);
    const oversizedToken = makeToken({ ...baseClaims, pad: hugePadding });
    assert.throws(() => {
      validateAuthHubToken(oversizedToken, validConfig);
    }, /exceeds maximum allowed size/i);
    console.log("✓ Oversized token (>4KB) rejected");
  }

  // 6. Negative: Expired token throws
  {
    const expiredToken = makeToken({ ...baseClaims, exp: now - 10 });
    assert.throws(() => {
      validateAuthHubToken(expiredToken, validConfig);
    }, /expired/i);
    console.log("✓ Expired token rejected");
  }

  // 7. Negative: Expiration exceeding 5 minutes throws
  {
    const longToken = makeToken({ ...baseClaims, exp: now + 600 });
    assert.throws(() => {
      validateAuthHubToken(longToken, validConfig);
    }, /exceeds maximum allowed 5 minutes/i);
    console.log("✓ Token exceeding 5 min validity rejected");
  }

  // 8. Negative: Mismatched sub throws
  {
    const wrongSubToken = makeToken({ ...baseClaims, sub: "33333333-3333-4333-a333-333333333333" });
    assert.throws(() => {
      validateAuthHubToken(wrongSubToken, validConfig);
    }, /sub claim mismatch/i);
    console.log("✓ Mismatched sub claim rejected");
  }

  // 9. Negative: Non-UUID sub throws
  {
    const nonUuidSubToken = makeToken({ ...baseClaims, sub: "not-a-uuid" });
    assert.throws(() => {
      validateAuthHubToken(nonUuidSubToken, { ...validConfig, testUserId: "not-a-uuid" });
    }, /valid UUID/i);
    console.log("✓ Non-UUID sub claim rejected");
  }

  // 10. Negative: Mismatched tenant_id throws
  {
    const wrongTenantToken = makeToken({ ...baseClaims, tenant_id: "44444444-4444-4444-a444-444444444444" });
    assert.throws(() => {
      validateAuthHubToken(wrongTenantToken, validConfig);
    }, /tenant_id claim mismatch/i);
    console.log("✓ Mismatched tenant_id claim rejected");
  }

  // 11. Negative: Non-UUID tenant_id throws
  {
    const nonUuidTenantToken = makeToken({ ...baseClaims, tenant_id: "bad-tenant" });
    assert.throws(() => {
      validateAuthHubToken(nonUuidTenantToken, { ...validConfig, testTenantId: "bad-tenant" });
    }, /valid UUID/i);
    console.log("✓ Non-UUID tenant_id claim rejected");
  }

  // 12. Negative: Invalid issuer throws
  {
    const wrongIssToken = makeToken({ ...baseClaims, iss: "fake-issuer" });
    assert.throws(() => {
      validateAuthHubToken(wrongIssToken, validConfig);
    }, /iss must be auth-hub/i);
    console.log("✓ Fake issuer rejected");
  }

  // 13. Negative: Invalid audience throws
  {
    const wrongAudToken = makeToken({ ...baseClaims, aud: ["untrusted-svc"] });
    assert.throws(() => {
      validateAuthHubToken(wrongAudToken, validConfig);
    }, /aud must include alt-backend/i);
    console.log("✓ Untrusted audience rejected");
  }

  // 14. Negative: Admin or wildcard role throws
  {
    const adminToken = makeToken({ ...baseClaims, role: "admin" });
    assert.throws(() => {
      validateAuthHubToken(adminToken, validConfig);
    }, /Admin\/wildcard roles are not permitted/i);

    const wildcardToken = makeToken({ ...baseClaims, role: "wildcard" });
    assert.throws(() => {
      validateAuthHubToken(wildcardToken, validConfig);
    }, /Admin\/wildcard roles are not permitted/i);
    console.log("✓ Admin and wildcard roles rejected");
  }

  // 15. Verify scenario feed-read-3000vu.js does NOT import or call generateJWT
  {
    const scenarioPath = path.resolve(__dirname, "../scenarios/feed-read-3000vu.js");
    const scenarioContent = fs.readFileSync(scenarioPath, "utf-8");
    assert.ok(
      !scenarioContent.includes("generateJWT"),
      "feed-read-3000vu.js must not import or call generateJWT"
    );
    assert.ok(
      scenarioContent.includes("getAuthHeaders"),
      "feed-read-3000vu.js must import getAuthHeaders"
    );
    console.log("✓ feed-read-3000vu.js clean of generateJWT and uses getAuthHeaders");
  }

  // 16. Valid short token fixture accepted for bounded cohort (<=180s total)
  {
    const validShortToken = makeToken({
      ...baseClaims,
      iat: now - 10,
      exp: now + 240, // 250s TTL window <= 300, remaining = 240s >= 180s (150s + 30s)
    });
    const claims = validateAuthHubToken(validShortToken, validConfig);
    assert.strictEqual(claims.sub, validUserId);
    const cohortClaims = validateCohortTokenAtInit(validShortToken, 150, validConfig);
    assert.strictEqual(cohortClaims.sub, validUserId);
    console.log("✓ Valid short token fixture accepted for cohort");
  }

  // 17. Negative: Missing or non-numeric iat claim throws
  {
    const noIatClaims = { ...baseClaims };
    delete noIatClaims.iat;
    const noIatToken = makeToken(noIatClaims);
    assert.throws(() => {
      validateAuthHubToken(noIatToken, validConfig);
    }, /missing or invalid iat/i);
    console.log("✓ Missing iat claim rejected");
  }

  // 18. Negative: Future iat beyond allowed clock skew throws
  {
    const futureIatToken = makeToken({ ...baseClaims, iat: now + 120, exp: now + 360 });
    assert.throws(() => {
      validateAuthHubToken(futureIatToken, validConfig);
    }, /Token iat is in the future/i);

    // Allowing acceptable small clock skew (e.g. 15s in future)
    const skewAllowedToken = makeToken({ ...baseClaims, iat: now + 15, exp: now + 280 });
    const skewClaims = validateAuthHubToken(skewAllowedToken, validConfig);
    assert.strictEqual(skewClaims.sub, validUserId);
    console.log("✓ Future iat (>60s skew) rejected, small skew accepted");
  }

  // 19. Negative: Long-issued token (exp - iat > 300) throws
  {
    const longIssuedToken = makeToken({ ...baseClaims, iat: now - 100, exp: now + 250 }); // 350s window > 300
    assert.throws(() => {
      validateAuthHubToken(longIssuedToken, validConfig);
    }, /validity duration.*exceeds maximum allowed 5 minutes/i);
    console.log("✓ Long-issued token (exp - iat > 300s) rejected");
  }

  // 20. Negative: Near-expiry token (< cohortDuration + 30s at init) throws
  {
    // Remaining is only 100s, less than required 180s for 150s cohort + 30s buffer
    const nearExpiryToken = makeToken({ ...baseClaims, iat: now - 100, exp: now + 100 });
    assert.throws(() => {
      validateCohortTokenAtInit(nearExpiryToken, 150, validConfig);
    }, /remaining validity.*insufficient/i);
    console.log("✓ Near-expiry token (< cohort duration + 30s buffer) rejected at init");
  }

  // 21. Negative: Expired token at request time throws (fail-closed)
  {
    const expiredToken = makeToken({ ...baseClaims, iat: now - 350, exp: now - 10 });
    assert.throws(() => {
      getAuthHeaders(expiredToken);
    }, /Token expired/i);
    console.log("✓ Expired token rejected at request time (fail-closed)");
  }

  // 22. Scenario stages verification: sum + gracefulRampDown <= 180s budget, peak 3000 VU achieved before budget
  {
    const scenarioPath = path.resolve(__dirname, "../scenarios/feed-read-3000vu.js");
    const scenarioContent = fs.readFileSync(scenarioPath, "utf-8");

    // Verify exported cohort constants
    const cohortDurMatch = scenarioContent.match(/export\s+const\s+COHORT_DURATION_SECONDS\s*=\s*(\d+)/);
    assert.ok(cohortDurMatch, "COHORT_DURATION_SECONDS must be exported");
    const cohortDuration = parseInt(cohortDurMatch[1], 10);
    assert.ok(cohortDuration <= 180, `COHORT_DURATION_SECONDS (${cohortDuration}s) must be <= 180s`);

    const reqRemMatch = scenarioContent.match(/export\s+const\s+REQUIRED_REMAINING_SECONDS\s*=\s*([^\n;]+)/);
    assert.ok(reqRemMatch, "REQUIRED_REMAINING_SECONDS must be exported");

    // Extract stages block
    const stagesMatch = scenarioContent.match(/stages:\s*\[([\s\S]*?)\]/);
    assert.ok(stagesMatch, "options.scenarios.feed_read must define stages");
    const stageLines = stagesMatch[1].match(/\{\s*duration:\s*["']([^"']+)["'],\s*target:\s*(\d+)\s*\}/g);
    assert.ok(stageLines && stageLines.length > 0, "Stages must contain duration and target");

    function parseDurationSec(durStr) {
      const m = durStr.match(/^(\d+)(s|m)$/);
      if (!m) throw new Error("Unknown duration format: " + durStr);
      const val = parseInt(m[1], 10);
      return m[2] === "m" ? val * 60 : val;
    }

    let totalStageSec = 0;
    let peakVU = 0;
    let peakAchievedAtSec = 0;

    for (const line of stageLines) {
      const itemMatch = line.match(/\{\s*duration:\s*["']([^"']+)["'],\s*target:\s*(\d+)\s*\}/);
      const dur = parseDurationSec(itemMatch[1]);
      const target = parseInt(itemMatch[2], 10);
      totalStageSec += dur;
      if (target > peakVU) {
        peakVU = target;
        peakAchievedAtSec = totalStageSec;
      }
    }

    const rampDownMatch = scenarioContent.match(/gracefulRampDown:\s*["']([^"']+)["']/);
    const gracefulSec = rampDownMatch ? parseDurationSec(rampDownMatch[1]) : 0;
    const totalBudgetSec = totalStageSec + gracefulSec;

    assert.strictEqual(peakVU, 3000, "Scenario peak must reach exactly 3000 VU");
    assert.ok(totalBudgetSec <= 180, `Total scenario duration + gracefulRampDown (${totalBudgetSec}s) must be <= 180s budget`);
    assert.ok(peakAchievedAtSec < totalBudgetSec, `Peak 3000 VU must be reached before total budget ends (${peakAchievedAtSec}s < ${totalBudgetSec}s)`);
    console.log(`✓ Scenario stages: sum=${totalStageSec}s + graceful=${gracefulSec}s = ${totalBudgetSec}s <= 180s; peak 3000 VU achieved at ${peakAchievedAtSec}s`);
  }

  console.log("All AuthHub JWT tests passed!");
}

run().catch((err) => {
  const category =
    err instanceof assert.AssertionError
      ? "AssertionError"
      : err instanceof TypeError
      ? "TypeError"
      : err instanceof RangeError
      ? "RangeError"
      : "ExecutionError";
  console.error("Test failed:", category);
  process.exit(1);
});
