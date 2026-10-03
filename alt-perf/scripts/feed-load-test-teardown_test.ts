import { assertEquals, assertFalse, assertStringIncludes } from "jsr:@std/assert";
import { teardownDatabaseAtomic } from "./feed-load-test-teardown.ts";
import { readManifest, type RunManifest, syntheticUserId } from "./manifest.ts";

Deno.test("teardownDatabaseAtomic generates exact single transaction", async () => {
  const tmpDir = await Deno.makeTempDir({ dir: "/tmp", prefix: "teardown_test_" });
  const synthUserId = syntheticUserId(1); // "00000000-0000-4000-a000-000000000001"

  const manifest: RunManifest = {
    version: "1",
    source: "feed-load-test-setup",
    runId: "test-run",
    createdAt: new Date().toISOString(),
    identities: [
      {
        kratosId: "11111111-1111-1111-1111-111111111111",
        syntheticUserId: synthUserId,
        email: "loadtest-0001@test.alt.local",
      },
    ],
    articles: [
      {
        articleId: "33333333-3333-3333-3333-333333333333",
        syntheticUserId: synthUserId,
        copiedFromUrl: "http://mock-rss-001:8080/feeds/1/rss.xml",
      },
    ],
    feeds: [
      {
        feedId: "44444444-4444-4444-4444-444444444444",
        mockRssUrl: "http://mock-rss-001:8080/feeds/1/rss.xml",
      },
    ],
    completedTeardownSteps: [],
    pendingExternalCleanup: [],
  };

  let executedSql = "";

  const mockExecSQL = async (
    _parts: string[],
    _lbl: string,
    _svc: string,
    _usr: string,
    _db: string,
    sql: string,
  ) => {
    executedSql = sql;
    return { success: true, stdout: "ok" };
  };

  const ok = await teardownDatabaseAtomic(
    manifest,
    `${tmpDir}/mock-path.json`,
    ["mock-compose"],
    "db",
    "user",
    "alt",
    false,
    mockExecSQL,
  );

  assertEquals(ok, true);

  assertStringIncludes(executedSql, "BEGIN;");
  assertStringIncludes(executedSql, "COMMIT;");
  assertStringIncludes(executedSql, "SET LOCAL statement_timeout");
  assertStringIncludes(executedSql, "SET LOCAL lock_timeout");
  assertStringIncludes(
    executedSql,
    "LOCK TABLE feed_links, feeds, user_feed_subscriptions, favorite_feeds, read_status, articles, article_heads, article_summaries IN SHARE ROW EXCLUSIVE MODE;",
  );
  assertStringIncludes(executedSql, "DO $$");
  assertStringIncludes(
    executedSql,
    "RAISE EXCEPTION 'Ownership preflight failed: Foreign article owner detected in DB';",
  );
  assertStringIncludes(
    executedSql,
    "RAISE EXCEPTION 'Ownership preflight failed: Foreign shared subscription dependency detected';",
  );
  assertStringIncludes(
    executedSql,
    "DELETE FROM articles WHERE id IN ('33333333-3333-3333-3333-333333333333')",
  );
  assertStringIncludes(
    executedSql,
    `DELETE FROM user_feed_subscriptions WHERE user_id IN ('${synthUserId}')`,
  );
  assertStringIncludes(
    executedSql,
    "RAISE EXCEPTION 'Rollback: target articles not absent after delete';",
  );
  assertStringIncludes(
    executedSql,
    "RAISE EXCEPTION 'Rollback: target feeds not absent after delete';",
  );
  assertStringIncludes(
    executedSql,
    "RAISE EXCEPTION 'Rollback: target subscriptions not absent after delete';",
  );

  const saved = await readManifest(`${tmpDir}/mock-path.json`);
  assertEquals(saved?.completedTeardownSteps.includes("db-teardown"), true);
  assertEquals(saved?.version, "1");
  assertEquals(saved?.source, "feed-load-test-setup");

  // Verify all-empty early return
  manifest.identities = [];
  manifest.articles = [];
  manifest.feeds = [];
  const ok2 = await teardownDatabaseAtomic(
    manifest,
    `${tmpDir}/mock-path2.json`,
    ["mock-compose"],
    "db",
    "user",
    "alt",
    false,
    mockExecSQL,
  );
  assertEquals(ok2, true);

  await Deno.remove(tmpDir, { recursive: true });
});

Deno.test("teardownDatabaseAtomic article-only: partially empty SQL fallback without outer parens and exact owner union", async () => {
  const tmpDir = await Deno.makeTempDir({ dir: "/tmp", prefix: "teardown_art_test_" });
  const synthUser = syntheticUserId(1); // "00000000-0000-4000-a000-000000000001" (real valid reserved synthetic UUID)
  const articleId = "33333333-3333-3333-3333-333333333333";

  const manifest: RunManifest = {
    version: "1",
    source: "feed-load-test-setup",
    runId: "article-only-run",
    createdAt: new Date().toISOString(),
    identities: [], // article-only: no identities
    articles: [
      {
        articleId,
        syntheticUserId: synthUser,
        copiedFromUrl: "http://mock-rss-001:8080/feeds/1/rss.xml",
      },
    ],
    feeds: [], // empty feeds
    completedTeardownSteps: [],
    pendingExternalCleanup: [],
  };

  let executedSql = "";
  const mockExecSQL = async (
    _parts: string[],
    _lbl: string,
    _svc: string,
    _usr: string,
    _db: string,
    sql: string,
  ) => {
    executedSql = sql;
    return { success: true, stdout: "ok" };
  };

  const mPath = `${tmpDir}/mock-path-art.json`;
  const ok = await teardownDatabaseAtomic(
    manifest,
    mPath,
    ["mock-compose"],
    "db",
    "user",
    "alt",
    false,
    mockExecSQL,
  );

  assertEquals(ok, true);
  // Atomic ownedUserIds matches preflight by including article syntheticUserId
  assertStringIncludes(executedSql, `a.user_id NOT IN ('${synthUser}')`);
  assertStringIncludes(executedSql, `a.id IN ('${articleId}')`);

  // Partially empty fallbacks MUST NOT contain outer parentheses
  assertStringIncludes(executedSql, "f.id IN (SELECT NULL::uuid WHERE FALSE)");
  assertStringIncludes(executedSql, "fl.url NOT IN (SELECT NULL::text WHERE FALSE)");
  assertStringIncludes(executedSql, "fl.url IN (SELECT NULL::text WHERE FALSE)");
  assertStringIncludes(executedSql, "ff.feed_id IN (SELECT NULL::uuid WHERE FALSE)");
  assertStringIncludes(executedSql, "rs.feed_id IN (SELECT NULL::uuid WHERE FALSE)");
  assertStringIncludes(executedSql, "a.feed_id IN (SELECT NULL::uuid WHERE FALSE)");

  // Strict check: NO double parentheses around scalar subquery
  assertFalse(executedSql.includes("((SELECT NULL::uuid WHERE FALSE))"));
  assertFalse(executedSql.includes("((SELECT NULL::text WHERE FALSE))"));

  // No zero UUID or empty string sentinel
  assertFalse(executedSql.includes("'00000000-0000-0000-0000-000000000000'"));

  // Deletions & post-delete verification
  assertStringIncludes(executedSql, `DELETE FROM articles WHERE id IN ('${articleId}')`);
  assertStringIncludes(
    executedSql,
    "DELETE FROM feeds WHERE id IN (SELECT NULL::uuid WHERE FALSE)",
  );
  assertStringIncludes(executedSql, "Rollback: target articles not absent after delete");

  // Roundtrip enum verification via readManifest
  const saved = await readManifest(mPath);
  assertEquals(saved?.completedTeardownSteps.includes("db-teardown"), true);
  assertEquals(saved?.version, "1");
  assertEquals(saved?.source, "feed-load-test-setup");

  await Deno.remove(tmpDir, { recursive: true });
});

Deno.test("teardownDatabaseAtomic feed-only: partially empty SQL fallback and foreign owner guard with db-teardown enum roundtrip", async () => {
  const tmpDir = await Deno.makeTempDir({ dir: "/tmp", prefix: "teardown_feed_test_" });
  const feedId = "44444444-4444-4444-4444-444444444444";
  const mockUrl = "http://mock-rss-001:8080/feeds/1/rss.xml";

  const manifest: RunManifest = {
    version: "1",
    source: "feed-load-test-setup",
    runId: "feed-only-run",
    createdAt: new Date().toISOString(),
    identities: [],
    articles: [],
    feeds: [
      {
        feedId,
        mockRssUrl: mockUrl,
      },
    ],
    completedTeardownSteps: [],
    pendingExternalCleanup: [],
  };

  let executedSql = "";
  const mockExecSQL = async (
    _parts: string[],
    _lbl: string,
    _svc: string,
    _usr: string,
    _db: string,
    sql: string,
  ) => {
    executedSql = sql;
    return { success: true, stdout: "ok" };
  };

  const mPath = `${tmpDir}/mock-path-feed.json`;
  const ok = await teardownDatabaseAtomic(
    manifest,
    mPath,
    ["mock-compose"],
    "db",
    "user",
    "alt",
    false,
    mockExecSQL,
  );

  assertEquals(ok, true);

  // userIdsSql is empty set subquery; NOT IN (SELECT NULL::uuid WHERE FALSE) evaluates to TRUE
  // so any foreign subscription or dependency correctly triggers foreign guard!
  assertStringIncludes(executedSql, "ufs.user_id NOT IN (SELECT NULL::uuid WHERE FALSE)");
  assertStringIncludes(executedSql, "ff.user_id NOT IN (SELECT NULL::uuid WHERE FALSE)");
  assertStringIncludes(executedSql, "rs.user_id NOT IN (SELECT NULL::uuid WHERE FALSE)");
  assertStringIncludes(executedSql, "a.user_id NOT IN (SELECT NULL::uuid WHERE FALSE)");

  // Target feed is in the IN clause
  assertStringIncludes(executedSql, `f.id IN ('${feedId}')`);
  assertStringIncludes(executedSql, `fl.url NOT IN ('${mockUrl}')`);
  assertStringIncludes(executedSql, `fl.url IN ('${mockUrl}')`);

  // Target articles fallback
  assertStringIncludes(executedSql, "a.id IN (SELECT NULL::uuid WHERE FALSE)");

  // Strict check: NO double parentheses around scalar subquery
  assertFalse(executedSql.includes("((SELECT NULL::uuid WHERE FALSE))"));
  assertFalse(executedSql.includes("((SELECT NULL::text WHERE FALSE))"));

  // Deletions & post-delete verification
  assertStringIncludes(executedSql, `DELETE FROM feeds WHERE id IN ('${feedId}')`);
  assertStringIncludes(
    executedSql,
    "DELETE FROM articles WHERE id IN (SELECT NULL::uuid WHERE FALSE)",
  );
  assertStringIncludes(executedSql, "Rollback: target feeds not absent after delete");

  // Roundtrip enum verification via readManifest
  const saved = await readManifest(mPath);
  assertEquals(saved?.completedTeardownSteps.includes("db-teardown"), true);
  assertEquals(saved?.version, "1");
  assertEquals(saved?.source, "feed-load-test-setup");

  await Deno.remove(tmpDir, { recursive: true });
});

Deno.test("teardownDatabaseAtomic exact matching owner union: identities and articles", async () => {
  const tmpDir = await Deno.makeTempDir({ dir: "/tmp", prefix: "teardown_union_test_" });
  const synthUser1 = syntheticUserId(1); // "00000000-0000-4000-a000-000000000001"
  const synthUser2 = syntheticUserId(2); // "00000000-0000-4000-a000-000000000002"
  const kratosUser3 = "33333333-3333-3333-3333-333333333333";

  const manifest: RunManifest = {
    version: "1",
    source: "feed-load-test-setup",
    runId: "union-run",
    createdAt: new Date().toISOString(),
    identities: [
      {
        kratosId: "11111111-1111-1111-1111-111111111111",
        syntheticUserId: synthUser1,
        email: "loadtest-0001@test.alt.local",
      },
      {
        kratosId: kratosUser3,
        syntheticUserId: "", // empty, should fallback to kratosId
        email: "loadtest-0002@test.alt.local",
      },
    ],
    articles: [
      {
        articleId: "55555555-5555-5555-5555-555555555555",
        syntheticUserId: synthUser2, // from article
        copiedFromUrl: "http://mock-rss-001:8080/feeds/1/rss.xml",
      },
    ],
    feeds: [],
    completedTeardownSteps: [],
    pendingExternalCleanup: [],
  };

  let executedSql = "";
  const mockExecSQL = async (
    _parts: string[],
    _lbl: string,
    _svc: string,
    _usr: string,
    _db: string,
    sql: string,
  ) => {
    executedSql = sql;
    return { success: true, stdout: "ok" };
  };

  const mPath = `${tmpDir}/mock-path-union.json`;
  const ok = await teardownDatabaseAtomic(
    manifest,
    mPath,
    ["mock-compose"],
    "db",
    "user",
    "alt",
    false,
    mockExecSQL,
  );

  assertEquals(ok, true);
  // All 3 valid UUID owners are in the userIdsSql list
  assertStringIncludes(executedSql, `'${synthUser1}'`);
  assertStringIncludes(executedSql, `'${synthUser2}'`);
  assertStringIncludes(executedSql, `'${kratosUser3}'`);

  await Deno.remove(tmpDir, { recursive: true });
});
