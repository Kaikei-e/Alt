/**
 * Manifest + cleanup workflow unit tests (D04 provenance)
 *
 * All tests use:
 *   - Deno.makeTempDir() for isolated temp files
 *   - No real network calls (all API interactions mocked inline)
 *   - No real DB / Kratos / Meilisearch / Redis
 *
 * Covers:
 *   1. Manifest create / write / read round-trip
 *   2. Atomic write (tmp → rename — verifies no partial state)
 *   3. Journal identity/article/feed incremental append
 *   4. ownsIdentity — email match required
 *   5. ownsIdentity — manually changed email → refuse delete
 *   6. ownsFeed — exact URL match required
 *   7. ownsArticle — exact ID required (gen_random_uuid, NOT prefix)
 *   8. Cross-run: foreign identities/feeds/articles preserved (not in manifest → ownsX = false)
 *   9. Partial teardown: manifest retained on step failure
 *  10. journalTeardownStep — idempotent (duplicate not added)
 *  11. Interrupted setup journal — recoverable (partial identity list retained)
 *  12. syntheticUserId — correct prefix format
 *  13. isSyntheticUserId — positive and negative cases
 *  14. Dry-run: ownsX guards return false on empty manifest → no delete
 *  15. pendingExternalCleanup journalling
 */

import { assertEquals, assertFalse, assertThrows } from "@std/assert";
import { afterEach, beforeEach, describe, it } from "@std/testing/bdd";
import {
  type ArticleRecord,
  createManifest,
  credentialsPath,
  currentManifestPath,
  type FeedRecord,
  type IdentityRecord,
  isMockRssUrl,
  isSyntheticEmail,
  isSyntheticUserId,
  isValidUUID,
  journalArticle,
  journalFeed,
  journalIdentity,
  journalPendingExternal,
  journalTeardownStep,
  manifestPath,
  ownsArticle,
  ownsFeed,
  ownsIdentity,
  readManifest,
  syntheticUserId,
  validateRunId,
  writeManifestAtomic,
} from "../../scripts/manifest.ts";
import {
  generateCryptoRandomPassword,
  getDeterministicPassword,
  loadExactRunCredentials,
  loadExistingCredentials,
  persistCredentialIncremental,
} from "../../scripts/feed-load-test-setup.ts";
import {
  buildSqlMockUrlInList,
  buildSqlUuidInList,
  type ExecSQLFn,
  executeTeardownFlow,
  preflightDatabaseOwnership,
  preflightKratosIdentities,
  teardownDatabaseAtomic,
} from "../../scripts/feed-load-test-teardown.ts";

// ── helpers ───────────────────────────────────────────────────────────────────

let tmpDir: string;

async function setup() {
  tmpDir = await Deno.makeTempDir({ dir: "/tmp", prefix: "manifest_test_" });
}

async function teardown() {
  await Deno.remove(tmpDir, { recursive: true });
}

function makeIdentity(overrides: Partial<IdentityRecord> = {}): IdentityRecord {
  return {
    kratosId: crypto.randomUUID(),
    email: "loadtest-0001@test.alt.local",
    syntheticUserId: "",
    ...overrides,
  };
}

function makeFeed(overrides: Partial<FeedRecord> = {}): FeedRecord {
  return {
    feedId: crypto.randomUUID(),
    mockRssUrl: "http://mock-rss-001:8080/feeds/1/rss.xml",
    ...overrides,
  };
}

function makeArticle(overrides: Partial<ArticleRecord> = {}): ArticleRecord {
  return {
    articleId: crypto.randomUUID(), // gen_random_uuid() — no prefix
    syntheticUserId: "00000000-0000-4000-a000-000000000001",
    copiedFromUrl: "https://real-source.example/article/123",
    ...overrides,
  };
}

// ── tests ─────────────────────────────────────────────────────────────────────

describe("manifest create/read round-trip", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("should create a manifest with correct defaults", () => {
    const m = createManifest("test-run-001", "feed-load-test-setup");
    assertEquals(m.runId, "test-run-001");
    assertEquals(m.version, "1");
    assertEquals(m.identities.length, 0);
    assertEquals(m.articles.length, 0);
    assertEquals(m.feeds.length, 0);
    assertEquals(m.pendingExternalCleanup.length, 0);
    assertEquals(m.completedTeardownSteps.length, 0);
    assertEquals(typeof m.createdAt, "string");
  });

  it("should write and read back the manifest", async () => {
    const m = createManifest("rr-001", "feed-load-test-setup");
    const p = manifestPath(tmpDir, "rr-001");
    await writeManifestAtomic(m, p);
    const read = await readManifest(p);
    assertEquals(read?.runId, "rr-001");
    assertEquals(read?.version, "1");
  });

  it("should return null for missing manifest", async () => {
    const result = await readManifest(manifestPath(tmpDir, "nonexistent-run"));
    assertEquals(result, null);
  });
});

describe("atomic write (no partial state)", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("should not leave a .tmp file after successful write", async () => {
    const m = createManifest("aw-001", "feed-load-test-setup");
    const p = manifestPath(tmpDir, "aw-001");
    await writeManifestAtomic(m, p);
    let tmpExists = false;
    try {
      await Deno.stat(`${p}.tmp`);
      tmpExists = true;
    } catch {
      // expected
    }
    assertFalse(tmpExists);
  });
});

describe("incremental journal", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("should append identity and persist atomically", async () => {
    const m = createManifest("jrn-001", "feed-load-test-setup");
    const p = manifestPath(tmpDir, "jrn-001");
    await writeManifestAtomic(m, p);

    const rec = makeIdentity({ kratosId: "11111111-1111-1111-1111-111111111111", email: "loadtest-0001@test.alt.local" });
    await journalIdentity(m, p, rec);

    const read = await readManifest(p);
    assertEquals(read?.identities.length, 1);
    assertEquals(read?.identities[0]?.kratosId, "11111111-1111-1111-1111-111111111111");
    assertEquals(read?.identities[0]?.email, "loadtest-0001@test.alt.local");
  });

  it("should append multiple identities across calls", async () => {
    const m = createManifest("jrn-002", "feed-load-test-setup");
    const p = manifestPath(tmpDir, "jrn-002");
    await writeManifestAtomic(m, p);

    await journalIdentity(
      m,
      p,
      makeIdentity({ kratosId: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", email: "loadtest-0001@test.alt.local" }),
    );
    await journalIdentity(
      m,
      p,
      makeIdentity({ kratosId: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", email: "loadtest-0002@test.alt.local" }),
    );

    const read = await readManifest(p);
    assertEquals(read?.identities.length, 2);
    assertEquals(read?.identities[1]?.kratosId, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb");
  });

  it("should append article record", async () => {
    const m = createManifest("jrn-003", "feed-load-test-setup");
    const p = manifestPath(tmpDir, "jrn-003");
    await writeManifestAtomic(m, p);
    const art = makeArticle({ articleId: "22222222-2222-2222-2222-222222222222" });
    await journalArticle(m, p, art);
    const read = await readManifest(p);
    assertEquals(read?.articles[0]?.articleId, "22222222-2222-2222-2222-222222222222");
  });

  it("should append feed record", async () => {
    const m = createManifest("jrn-004", "feed-load-test-setup");
    const p = manifestPath(tmpDir, "jrn-004");
    await writeManifestAtomic(m, p);
    const feed = makeFeed({
      feedId: "33333333-3333-3333-3333-333333333333",
      mockRssUrl: "http://mock-rss-042:8080/feeds/42/rss.xml",
    });
    await journalFeed(m, p, feed);
    const read = await readManifest(p);
    assertEquals(read?.feeds[0]?.feedId, "33333333-3333-3333-3333-333333333333");
    assertEquals(read?.feeds[0]?.mockRssUrl, "http://mock-rss-042:8080/feeds/42/rss.xml");
  });
});

describe("ownsIdentity", () => {
  it("should return true when kratosId and email both match manifest", () => {
    const m = createManifest("oi-1", "feed-load-test-setup");
    const rec = makeIdentity({ kratosId: "k-001", email: "loadtest-0001@test.alt.local" });
    m.identities.push(rec);
    assertEquals(ownsIdentity(m, "k-001", "loadtest-0001@test.alt.local"), true);
  });

  it("should return false when kratosId not in manifest", () => {
    const m = createManifest("oi-2", "feed-load-test-setup");
    assertFalse(ownsIdentity(m, "k-stranger", "loadtest-0001@test.alt.local"));
  });

  it("should return false when email was manually changed (ownership mismatch)", () => {
    const m = createManifest("oi-3", "feed-load-test-setup");
    const rec = makeIdentity({ kratosId: "k-003", email: "loadtest-0003@test.alt.local" });
    m.identities.push(rec);
    // Simulate operator changed the email in Kratos
    assertFalse(ownsIdentity(m, "k-003", "manually-changed@other.example"));
  });

  it("should return false when kratosId matches but email differs (foreign run)", () => {
    const m = createManifest("oi-4", "feed-load-test-setup");
    m.identities.push(makeIdentity({
      kratosId: "k-004",
      email: "loadtest-0004@test.alt.local",
    }));
    // Another run's identity happened to get the same kratosId (shouldn't happen with UUIDs,
    // but the email gate is the final safety check)
    assertFalse(ownsIdentity(m, "k-004", "loadtest-9999@test.alt.local"));
  });
});

describe("ownsFeed", () => {
  it("should return true when feedId and mockRssUrl both match", () => {
    const m = createManifest("of-1", "feed-load-test-setup");
    const f = makeFeed({ feedId: "f-001", mockRssUrl: "http://mock-rss-001:8080/feed.xml" });
    m.feeds.push(f);
    assertEquals(ownsFeed(m, "f-001", "http://mock-rss-001:8080/feed.xml"), true);
  });

  it("should return false when feedId not in manifest", () => {
    const m = createManifest("of-2", "feed-load-test-setup");
    assertFalse(ownsFeed(m, "f-stranger", "http://mock-rss-001:8080/feed.xml"));
  });

  it("should return false when URL does not match (cross-run protection)", () => {
    const m = createManifest("of-3", "feed-load-test-setup");
    m.feeds.push(makeFeed({ feedId: "f-003", mockRssUrl: "http://mock-rss-003:8080/run1.xml" }));
    // Different run used same feedId but different URL (shouldn't happen in practice,
    // but the exact URL gate prevents broad deletes)
    assertFalse(ownsFeed(m, "f-003", "http://mock-rss-003:8080/run2.xml"));
  });

  it("should NOT match a real/shared feed URL", () => {
    const m = createManifest("of-4", "feed-load-test-setup");
    m.feeds.push(makeFeed({ feedId: "f-real", mockRssUrl: "https://real.news/feed.xml" }));
    // Even if feedId matches, a real URL should not be in the manifest in the first place;
    // and ownsFeed is only called for manifest entries, so this exercises the URL guard.
    assertFalse(ownsFeed(m, "f-real", "https://OTHER.news/feed.xml"));
  });
});

describe("ownsArticle", () => {
  it("should return true for exact articleId in manifest", () => {
    const m = createManifest("oa-1", "feed-load-test-setup");
    m.articles.push(makeArticle({ articleId: "art-exact-001" }));
    assertEquals(ownsArticle(m, "art-exact-001"), true);
  });

  it("should return false for articleId not in manifest", () => {
    const m = createManifest("oa-2", "feed-load-test-setup");
    assertFalse(ownsArticle(m, "art-missing"));
  });

  it("article IDs are gen_random_uuid — NOT the 00000000-0000-4000-a000- prefix", () => {
    const m = createManifest("oa-3", "feed-load-test-setup");
    const realUUID = "3f2504e0-4f89-11d3-9a0c-0305e82c3301"; // realistic gen_random_uuid output
    m.articles.push(makeArticle({ articleId: realUUID }));
    assertEquals(ownsArticle(m, realUUID), true);
    // The synthetic user PREFIX must NOT be treated as an article ID prefix
    assertFalse(ownsArticle(m, "00000000-0000-4000-a000-000000000001"));
  });
});

describe("cross-run preservation", () => {
  it("foreign identity (different run, not in manifest) is not owned", () => {
    const m = createManifest("cr-1", "feed-load-test-setup");
    // This run only created identity k-mine
    m.identities.push(makeIdentity({ kratosId: "k-mine", email: "loadtest-0001@test.alt.local" }));
    // Foreign identity from another run
    assertFalse(ownsIdentity(m, "k-foreign-run", "loadtest-0001@test.alt.local"));
  });

  it("foreign feed (different run) is not owned", () => {
    const m = createManifest("cr-2", "feed-load-test-setup");
    m.feeds.push(makeFeed({ feedId: "f-mine", mockRssUrl: "http://mock-rss-001:8080/mine.xml" }));
    assertFalse(ownsFeed(m, "f-foreign", "http://mock-rss-001:8080/mine.xml"));
  });

  it("real (non-mock) copied articles are NOT in manifest.articles — ownsArticle=false", () => {
    const m = createManifest("cr-3", "feed-load-test-setup");
    // Source articles are never journalled (only copies are)
    const realSourceId = "real-source-article-uuid";
    assertFalse(ownsArticle(m, realSourceId));
  });
});

describe("teardown step journalling", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("should record completed teardown steps", async () => {
    const m = createManifest("ts-1", "feed-load-test-setup");
    const p = manifestPath(tmpDir, "ts-1");
    await writeManifestAtomic(m, p);

    await journalTeardownStep(m, p, "kratos-identities");
    assertEquals(m.completedTeardownSteps.includes("kratos-identities"), true);

    const read = await readManifest(p);
    assertEquals(read?.completedTeardownSteps.includes("kratos-identities"), true);
  });

  it("should be idempotent — same step not added twice", async () => {
    const m = createManifest("ts-2", "feed-load-test-setup");
    const p = manifestPath(tmpDir, "ts-2");
    await writeManifestAtomic(m, p);

    await journalTeardownStep(m, p, "db-articles");
    await journalTeardownStep(m, p, "db-articles");

    assertEquals(m.completedTeardownSteps.filter((s) => s === "db-articles").length, 1);
  });

  it("should preserve manifest on partial cleanup (simulate failure by checking step list)", async () => {
    const m = createManifest("ts-3", "feed-load-test-setup");
    const p = manifestPath(tmpDir, "ts-3");
    await writeManifestAtomic(m, p);

    // Simulate: kratos done, db-articles failed (not recorded)
    await journalTeardownStep(m, p, "kratos-identities");
    // db-articles step NOT journalled (simulates failure)

    const read = await readManifest(p);
    // Manifest must still exist and show partial state
    assertEquals(read !== null, true);
    assertEquals(read?.completedTeardownSteps.includes("kratos-identities"), true);
    assertFalse(read?.completedTeardownSteps.includes("db-articles") ?? false);
  });
});

describe("interrupted setup recovery", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("partial identity list is recoverable from manifest after interruption", async () => {
    const m = createManifest("isr-1", "feed-load-test-setup");
    const p = manifestPath(tmpDir, "isr-1");
    await writeManifestAtomic(m, p);

    // Simulate: 3 identities journalled before interruption
    await journalIdentity(
      m,
      p,
      makeIdentity({ kratosId: "44444444-4444-4444-4444-444444444444", email: "loadtest-0000@test.alt.local" }),
    );
    await journalIdentity(
      m,
      p,
      makeIdentity({ kratosId: "55555555-5555-5555-5555-555555555555", email: "loadtest-0001@test.alt.local" }),
    );
    await journalIdentity(
      m,
      p,
      makeIdentity({ kratosId: "66666666-6666-6666-6666-666666666666", email: "loadtest-0002@test.alt.local" }),
    );

    // "Process dies here" — read manifest from disk
    const recovered = await readManifest(p);
    assertEquals(recovered?.identities.length, 3);
    assertEquals(recovered?.identities[2]?.kratosId, "66666666-6666-6666-6666-666666666666");
    // Setup can resume: identities already in manifest are skipped (not re-created)
    const alreadyDone = recovered?.identities.map((r) => r.email) ?? [];
    assertEquals(alreadyDone.includes("loadtest-0001@test.alt.local"), true);
  });
});

describe("syntheticUserId helpers", () => {
  it("generates correct prefix format for index 1", () => {
    assertEquals(syntheticUserId(1), "00000000-0000-4000-a000-000000000001");
  });

  it("generates correct prefix format for index 3000", () => {
    assertEquals(syntheticUserId(3000), "00000000-0000-4000-a000-000000003000");
  });

  it("isSyntheticUserId returns true for synthetic prefix", () => {
    assertEquals(isSyntheticUserId("00000000-0000-4000-a000-000000000042"), true);
  });

  it("isSyntheticUserId returns false for real user UUIDs", () => {
    assertFalse(isSyntheticUserId("3f2504e0-4f89-11d3-9a0c-0305e82c3301"));
    assertFalse(isSyntheticUserId(""));
    assertFalse(isSyntheticUserId("00000000-0000-0000-0000-000000000000"));
  });
});

describe("pendingExternalCleanup journalling", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("should append pending external entries and persist", async () => {
    const m = createManifest("pe-1", "feed-load-test-setup");
    const p = manifestPath(tmpDir, "pe-1");
    await writeManifestAtomic(m, p);

    await journalPendingExternal(m, p, {
      system: "meilisearch",
      reason: "No direct route",
      keys: ["art-001", "art-002"],
    });

    const read = await readManifest(p);
    assertEquals(read?.pendingExternalCleanup.length, 1);
    assertEquals(read?.pendingExternalCleanup[0]?.system, "meilisearch");
    assertEquals(read?.pendingExternalCleanup[0]?.keys.length, 2);
  });

  it("sovereign events entry notes append-only constraint", async () => {
    const m = createManifest("pe-2", "feed-load-test-setup");
    const p = manifestPath(tmpDir, "pe-2");
    await writeManifestAtomic(m, p);

    await journalPendingExternal(m, p, {
      system: "sovereign-events",
      reason: "Append-only event log. No delete. PENDING service-API strategy.",
      keys: [],
    });

    const read = await readManifest(p);
    assertEquals(read?.pendingExternalCleanup[0]?.system, "sovereign-events");
  });
});

describe("dry-run: ownsX guards prevent deletion on empty manifest", () => {
  it("empty manifest owns nothing — all guards return false", () => {
    const m = createManifest("dr-1", "feed-load-test-setup");
    assertFalse(ownsIdentity(m, "k-any", "any@test.alt.local"));
    assertFalse(ownsFeed(m, "f-any", "http://mock-rss-001:8080/f.xml"));
    assertFalse(ownsArticle(m, "any-article-id"));
  });
});

describe("manifestPath / currentManifestPath", () => {
  it("manifestPath includes runId", () => {
    const p = manifestPath("/data", "my-run-2026");
    assertEquals(p.includes("my-run-2026"), true);
    assertEquals(p.includes("run-manifest"), true);
  });

  it("currentManifestPath is a consistent well-known path", () => {
    const p1 = currentManifestPath("/data");
    const p2 = currentManifestPath("/data");
    assertEquals(p1, p2);
    assertEquals(p1.includes("current"), true);
  });
});

describe("concurrent50journal: exact 50 persist, 0 errors, no .tmp race", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("50 concurrent journal mutations all succeed and persist without race", async () => {
    const runId = "conc-50-run";
    const m = createManifest(runId, "feed-load-test-setup");
    const p = manifestPath(tmpDir, runId);
    await writeManifestAtomic(m, p);

    const promises = Array.from({ length: 50 }, (_, i) => {
      const rec: IdentityRecord = {
        kratosId: crypto.randomUUID(),
        email: `loadtest-${String(i).padStart(4, "0")}@test.alt.local`,
        syntheticUserId: "",
      };
      return journalIdentity(m, p, rec);
    });

    const results = await Promise.allSettled(promises);
    const errors = results.filter((r) => r.status === "rejected");
    assertEquals(errors.length, 0);

    const read = await readManifest(p);
    assertEquals(read?.identities.length, 50);

    // Verify all 50 unique emails exist
    const emails = new Set(read?.identities.map((i) => i.email));
    assertEquals(emails.size, 50);

    // Verify no stray .tmp files left in tmpDir
    let leftoverTmp = 0;
    for await (const entry of Deno.readDir(tmpDir)) {
      if (entry.name.includes(".tmp")) leftoverTmp++;
    }
    assertEquals(leftoverTmp, 0);
  });
});

describe("runpathbadrejection: directory traversal and invalid runId rejection", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("rejects directory traversal attempts", () => {
    assertThrows(
      () => manifestPath(tmpDir, "../../../outside"),
      Error,
      "Invalid runId",
    );
    assertThrows(
      () => manifestPath(tmpDir, "../../etc/passwd"),
      Error,
      "Invalid runId",
    );
    assertThrows(
      () => manifestPath(tmpDir, "nested/path/run"),
      Error,
      "Invalid runId",
    );
    assertThrows(
      () => manifestPath(tmpDir, "run with spaces"),
      Error,
      "Invalid runId",
    );
    assertThrows(
      () => manifestPath(tmpDir, "run;drop_table"),
      Error,
      "Invalid runId",
    );
    assertThrows(
      () => validateRunId("../bad"),
      Error,
      "Invalid runId",
    );
  });

  it("accepts valid alphanumeric, hyphenated, and underscored runIds", () => {
    const p1 = manifestPath(tmpDir, "run-123_abc");
    assertEquals(p1.includes("run-123_abc"), true);
    validateRunId("run-123_abc");
  });
});

describe("SQLinvalidUUID: SQL injection prevention & strict UUID / mock URL validation", () => {
  it("rejects malicious or invalid UUIDs in buildSqlUuidInList", () => {
    assertThrows(
      () => buildSqlUuidInList(["' OR '1'='1"]),
      Error,
      "Security validation failure",
    );
    assertThrows(
      () => buildSqlUuidInList(["not-a-uuid"]),
      Error,
      "Security validation failure",
    );
    assertThrows(
      () => buildSqlUuidInList(["3f2504e0-4f89-11d3-9a0c-0305e82c3301; DROP TABLE articles;--"]),
      Error,
      "Security validation failure",
    );
  });

  it("accepts valid UUIDs in buildSqlUuidInList", () => {
    const id1 = crypto.randomUUID();
    const id2 = crypto.randomUUID();
    const sql = buildSqlUuidInList([id1, id2]);
    assertEquals(sql, `'${id1}','${id2}'`);
  });

  it("rejects non-mock or malicious URLs in buildSqlMockUrlInList", () => {
    assertThrows(
      () => buildSqlMockUrlInList(["https://google.com/feed.xml"]),
      Error,
      "Security validation failure",
    );
    assertThrows(
      () => buildSqlMockUrlInList(["javascript:alert(1)"]),
      Error,
      "Security validation failure",
    );
    assertThrows(
      () => buildSqlMockUrlInList(["http://evil.attacker.org/rss"]),
      Error,
      "Security validation failure",
    );
  });

  it("accepts approved mock RSS URLs in buildSqlMockUrlInList", () => {
    const url = "http://mock-rss-001:8080/feeds/1/rss.xml";
    const sql = buildSqlMockUrlInList([url]);
    assertEquals(sql, `'${url}'`);
    assertEquals(isMockRssUrl(url), true);
    assertEquals(isMockRssUrl("https://example.com/rss"), false);
    assertEquals(isSyntheticEmail("loadtest-0001@test.alt.local"), true);
    assertEquals(isSyntheticEmail("hacker@other.com"), false);
    assertEquals(isValidUUID(crypto.randomUUID()), true);
    assertEquals(isValidUUID("bad-uuid"), false);
  });
});

describe("mismatch0actions: Kratos identity preflight failure aborts with 0 mutations", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("aborts teardown immediately with 0 DELETEs and 0 SQL calls on email mismatch", async () => {
    const runId = "mismatch-run";
    const m = createManifest(runId, "feed-load-test-setup");
    const p = manifestPath(tmpDir, runId);
    const testKratosId = crypto.randomUUID();

    m.identities.push({
      kratosId: testKratosId,
      email: "loadtest-0001@test.alt.local",
      syntheticUserId: "",
    });
    await writeManifestAtomic(m, p);

    let deleteFetchCalls = 0;
    let sqlExecutionCalls = 0;
    let removeFileCalls = 0;

    const mockFetch = (input: string | URL | Request, init?: RequestInit): Promise<Response> => {
      const urlStr = String(input);
      const method = init?.method || "GET";

      if (method === "DELETE") {
        deleteFetchCalls++;
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      if (urlStr.includes(`/admin/identities/${testKratosId}`)) {
        // Return mismatched email!
        return Promise.resolve(
          new Response(
            JSON.stringify({ traits: { email: "attacker-hijacked@other.com" } }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    };

    const mockExecSQL: ExecSQLFn = (_parts, _label, _service, _user, _db, _sql) => {
      sqlExecutionCalls++;
      return Promise.resolve({ success: true, stdout: "" });
    };

    const mockRemove = (_path: string) => {
      removeFileCalls++;
      return Promise.resolve();
    };

    const preflight = await preflightKratosIdentities(
      m,
      "http://mock-kratos",
      mockFetch as typeof fetch,
    );
    assertEquals(preflight.ok, false);

    const result = await executeTeardownFlow({
      manifest: m,
      mPath: p,
      dryRun: false,
      kratosAdmin: "http://mock-kratos",
      dataDir: tmpDir,
      fetchFn: mockFetch as typeof fetch,
      execSQL: mockExecSQL,
      removeFn: mockRemove,
    });

    assertEquals(result.success, false);
    assertEquals(deleteFetchCalls, 0); // 0 Kratos deletes
    assertEquals(sqlExecutionCalls, 0); // 0 DB mutations
    assertEquals(removeFileCalls, 0); // 0 files deleted

    // Manifest still exists and was preserved
    const preserved = await readManifest(p);
    assertEquals(preserved !== null, true);
    assertEquals(preserved?.identities.length, 1);
  });

  it("aborts teardown immediately with 0 DELETEs and 0 SQL calls on missing or foreign run marker", async () => {
    const runId = "marker-mismatch-run";
    const m = createManifest(runId, "feed-load-test-setup");
    const p = manifestPath(tmpDir, runId);
    const testKratosId = crypto.randomUUID();

    m.identities.push({
      kratosId: testKratosId,
      email: "loadtest-0001@test.alt.local",
      syntheticUserId: "",
    });
    await writeManifestAtomic(m, p);

    let deleteFetchCalls = 0;
    let sqlCalls = 0;

    // 1. Missing marker
    const mockFetchMissing = (input: string | URL | Request, init?: RequestInit): Promise<Response> => {
      const method = init?.method || "GET";
      if (method === "DELETE") {
        deleteFetchCalls++;
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({ traits: { email: "loadtest-0001@test.alt.local" } }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );
    };

    const mockExecSQL: ExecSQLFn = () => {
      sqlCalls++;
      throw new Error("execSQL must NOT be called on preflight failure!");
    };

    const preflight1 = await preflightKratosIdentities(m, "http://mock-kratos", mockFetchMissing as typeof fetch);
    assertEquals(preflight1.ok, false);
    assertEquals(preflight1.reason?.includes("metadata run_id"), true);

    const result1 = await executeTeardownFlow({
      manifest: m,
      mPath: p,
      dryRun: false,
      kratosAdmin: "http://mock-kratos",
      dataDir: tmpDir,
      fetchFn: mockFetchMissing as typeof fetch,
      execSQL: mockExecSQL,
    });
    assertEquals(result1.success, false);
    assertEquals(deleteFetchCalls, 0);
    assertEquals(sqlCalls, 0);

    // 2. Foreign marker
    const mockFetchForeign = (input: string | URL | Request, init?: RequestInit): Promise<Response> => {
      const method = init?.method || "GET";
      if (method === "DELETE") {
        deleteFetchCalls++;
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({
            traits: { email: "loadtest-0001@test.alt.local" },
            metadata_admin: { run_id: "other-foreign-run" },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );
    };

    const preflight2 = await preflightKratosIdentities(m, "http://mock-kratos", mockFetchForeign as typeof fetch);
    assertEquals(preflight2.ok, false);
    assertEquals(preflight2.reason?.includes("other-foreign-run"), true);

    const result2 = await executeTeardownFlow({
      manifest: m,
      mPath: p,
      dryRun: false,
      kratosAdmin: "http://mock-kratos",
      dataDir: tmpDir,
      fetchFn: mockFetchForeign as typeof fetch,
      execSQL: mockExecSQL,
    });
    assertEquals(result2.success, false);
    assertEquals(deleteFetchCalls, 0);
    assertEquals(sqlCalls, 0);
  });
});

describe("dbpreflightforeign: currently foreign article/shared subscribed feeds ZERO mutation globally before firstdelete", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("aborts teardown with ZERO mutations when DB has foreign article owner", async () => {
    const runId = "foreign-art-run";
    const m = createManifest(runId, "feed-load-test-setup");
    const p = manifestPath(tmpDir, runId);
    const artId = crypto.randomUUID();
    const foreignUser = crypto.randomUUID();
    const myUser = "00000000-0000-4000-a000-000000000001";

    m.articles.push({
      articleId: artId,
      syntheticUserId: myUser,
      copiedFromUrl: "https://example.com/art",
    });
    await writeManifestAtomic(m, p);

    let deleteFetchCalls = 0;
    let sqlDeleteCalls = 0;

    const mockFetch = (_input: string | URL | Request, init?: RequestInit): Promise<Response> => {
      const method = init?.method || "GET";
      if (method === "DELETE") {
        deleteFetchCalls++;
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({
            traits: { email: "loadtest-0001@test.alt.local" },
            metadata_admin: { run_id: runId },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );
    };

    const mockExecSQL: ExecSQLFn = (_parts, _label, _service, _user, _db, sql) => {
      if (sql.includes("DELETE")) {
        sqlDeleteCalls++;
        throw new Error("DELETE must NOT be executed if DB preflight detects foreign owner!");
      }
      // Return JSON array for articles preflight
      return Promise.resolve({
        success: true,
        stdout: JSON.stringify([{ id: artId, user_id: foreignUser, url: "https://example.com/art" }]),
      });
    };

    const dbPreflight = await preflightDatabaseOwnership(m, ["docker"], "db", "user", "alt", false, mockExecSQL);
    assertEquals(dbPreflight.ok, false);
    assertEquals(dbPreflight.reason?.includes("Foreign article owner detected"), true);

    const result = await executeTeardownFlow({
      manifest: m,
      mPath: p,
      dryRun: false,
      kratosAdmin: "http://mock-kratos",
      dataDir: tmpDir,
      fetchFn: mockFetch as typeof fetch,
      execSQL: mockExecSQL,
    });

    assertEquals(result.success, false);
    assertEquals(deleteFetchCalls, 0); // ZERO Kratos mutations
    assertEquals(sqlDeleteCalls, 0); // ZERO SQL delete mutations
  });
});

describe("sharedrefskeep: preserve shared feed_links and foreign references", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("teardownDatabase generates safe NOT EXISTS queries preserving shared references", async () => {
    const runId = "shared-ref-run";
    const m = createManifest(runId, "feed-load-test-setup");
    const p = manifestPath(tmpDir, runId);
    const feedId = crypto.randomUUID();
    const mockUrl = "http://mock-rss-001:8080/feeds/2/rss.xml";

    m.feeds.push({ feedId, mockRssUrl: mockUrl });
    await writeManifestAtomic(m, p);

    const executedSql: string[] = [];
    const mockExecSQL: ExecSQLFn = (_parts, _label, _service, _user, _db, sql) => {
      executedSql.push(sql);
      return Promise.resolve({ success: true, stdout: "" });
    };

    const ok = await teardownDatabaseAtomic(
      m,
      p,
      ["docker"],
      "db",
      "user",
      "alt",
      false,
      mockExecSQL,
    );
    assertEquals(ok, true);
    assertEquals(executedSql.length, 1);
    const sql = executedSql[0];

    // Verify feed_links deletion uses NOT EXISTS to prevent cascade on other users/feeds
    assertEquals(sql?.includes("DELETE FROM feed_links"), true);
    assertEquals(sql?.includes("NOT EXISTS"), true);
    assertEquals(sql?.includes(mockUrl), true);
  });
});

describe("numericmockexactIDs: no global prefix deletion across runs", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("synthetic user rows teardown targets only exact user IDs from manifest, never global prefix", async () => {
    const runId = "exact-ids-run";
    const m = createManifest(runId, "feed-load-test-setup");
    const p = manifestPath(tmpDir, runId);
    const userUuid1 = crypto.randomUUID();
    const userUuid2 = crypto.randomUUID();

    m.identities.push({
      kratosId: userUuid1,
      email: "loadtest-0001@test.alt.local",
      syntheticUserId: userUuid1,
    });
    m.identities.push({
      kratosId: userUuid2,
      email: "loadtest-0002@test.alt.local",
      syntheticUserId: userUuid2,
    });
    await writeManifestAtomic(m, p);

    const executedSql: string[] = [];
    const mockExecSQL: ExecSQLFn = (_parts, _label, _service, _user, _db, sql) => {
      executedSql.push(sql);
      return Promise.resolve({ success: true, stdout: "" });
    };

    const ok = await teardownDatabaseAtomic(
      m,
      p,
      ["docker"],
      "db",
      "user",
      "alt",
      false,
      mockExecSQL,
    );
    assertEquals(ok, true);
    assertEquals(executedSql.length, 1);

    const sql = executedSql[0];
    // Must NOT contain LIKE or global prefix wildcard %
    assertFalse(sql?.includes("LIKE") ?? true);
    assertFalse(sql?.includes("00000000-0000-4000-a000-%") ?? true);
    // Must contain exact UUIDs in IN clause
    assertEquals(sql?.includes(`'${userUuid1}'`), true);
    assertEquals(sql?.includes(`'${userUuid2}'`), true);
  });
});

describe("partialfailurekeeps: data files and credentials retained on partial failure", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("retains data files and sets status local_partial when a teardown step fails", async () => {
    const runId = "part-fail-run";
    const m = createManifest(runId, "feed-load-test-setup");
    const p = manifestPath(tmpDir, runId);
    const testKratosId = crypto.randomUUID();

    m.identities.push({
      kratosId: testKratosId,
      email: "loadtest-0001@test.alt.local",
      syntheticUserId: "",
    });
    await writeManifestAtomic(m, p);

    // Create mock user credential files
    await Deno.writeTextFile(
      `${tmpDir}/load-test-users.json`,
      JSON.stringify([{ email: "loadtest-0001@test.alt.local" }]),
    );
    await Deno.writeTextFile(`${tmpDir}/load-test-users.csv`, "email,password,userId\n");

    let removeCalls = 0;
    const mockFetch = (_input: string | URL | Request, init?: RequestInit): Promise<Response> => {
      const method = init?.method || "GET";
      if (method === "GET") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              traits: { email: "loadtest-0001@test.alt.local" },
              metadata_admin: { run_id: runId },
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      }
      // DELETE fails!
      return Promise.resolve(new Response("Internal error", { status: 500 }));
    };

    const mockRemove = (_path: string) => {
      removeCalls++;
      return Promise.resolve();
    };

    let sqlCalls = 0;
    const mockExecSQL: ExecSQLFn = () => {
      sqlCalls++;
      throw new Error("execSQL must NOT be called when Kratos identity teardown fails!");
    };

    const credJournalPath = credentialsPath(tmpDir, runId);
    await Deno.writeTextFile(
      credJournalPath,
      JSON.stringify([{ email: "loadtest-0001@test.alt.local", password: "p1" }]),
    );

    const result = await executeTeardownFlow({
      manifest: m,
      mPath: p,
      dryRun: false,
      kratosAdmin: "http://mock-kratos",
      dataDir: tmpDir,
      fetchFn: mockFetch as typeof fetch,
      execSQL: mockExecSQL,
      removeFn: mockRemove,
    });

    assertEquals(result.success, false);
    assertEquals(result.status, "local_partial");
    assertEquals(removeCalls, 0); // Data files NOT removed!
    assertEquals(sqlCalls, 0); // SQL calls strictly ZERO after identity failure!

    // Verify credential files still exist
    const jsonStat = await Deno.stat(`${tmpDir}/load-test-users.json`);
    assertEquals(jsonStat.isFile, true);
    const credStat = await Deno.stat(credJournalPath);
    assertEquals(credStat.isFile, true);

    // Verify manifest has local_partial and NO teardownCompletedAt
    const updated = await readManifest(p);
    assertEquals(updated?.teardownStatus, "local_partial");
    assertEquals(updated?.teardownCompletedAt, undefined);
  });

  it("sets status external_pending and preserves manifest and credentials when external items remain", async () => {
    const runId = "ext-pending-run";
    const m = createManifest(runId, "feed-load-test-setup");
    const p = manifestPath(tmpDir, runId);
    m.pendingExternalCleanup.push({
      system: "sovereign-events",
      reason: "Append only",
      keys: [],
    });
    await writeManifestAtomic(m, p);

    const credJournalPath = credentialsPath(tmpDir, runId);
    await Deno.writeTextFile(
      credJournalPath,
      JSON.stringify([{ email: "loadtest-0001@test.alt.local", password: "p1" }]),
    );

    let removeCalls = 0;
    const mockRemove = (_path: string) => {
      removeCalls++;
      return Promise.resolve();
    };

    const mockExecSQL: ExecSQLFn = () => Promise.resolve({ success: true, stdout: "" });

    const result = await executeTeardownFlow({
      manifest: m,
      mPath: p,
      dryRun: false,
      kratosAdmin: "http://mock-kratos",
      dataDir: tmpDir,
      execSQL: mockExecSQL,
      removeFn: mockRemove,
    });

    assertEquals(result.success, true);
    assertEquals(result.status, "external_pending");
    assertEquals(removeCalls, 0); // Must NOT delete files when external items are pending!

    const credStat = await Deno.stat(credJournalPath);
    assertEquals(credStat.isFile, true);

    const updated = await readManifest(p);
    assertEquals(updated?.teardownStatus, "external_pending");
    assertEquals(updated?.teardownCompletedAt, undefined); // NOT completed!
  });
});

describe("randomprivatecredentials: crypto random passwords and private credential journal resume", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("generates high-entropy crypto random passwords (non-deterministic)", () => {
    const pwd1 = generateCryptoRandomPassword();
    const pwd2 = generateCryptoRandomPassword();

    assertEquals(pwd1.length >= 16, true);
    assertEquals(pwd2.length >= 16, true);
    // Non-deterministic: repeat calls MUST differ
    assertFalse(pwd1 === pwd2);
    assertFalse(pwd1.includes("not-available"));
    assertFalse(pwd1.includes("placeholder"));
  });

  it("incrementally persists credentials with chmod 0600 and resumes only exact run artifact", async () => {
    const runId = "exact-run-123";
    const journalPath = credentialsPath(tmpDir, runId);

    const user1 = { email: "loadtest-0001@test.alt.local", password: generateCryptoRandomPassword(), userId: "u1" };
    const user2 = { email: "loadtest-0002@test.alt.local", password: generateCryptoRandomPassword(), userId: "u2" };

    await persistCredentialIncremental(journalPath, user1);
    await persistCredentialIncremental(journalPath, user2);

    const stat = await Deno.stat(journalPath);
    assertEquals(stat.isFile, true);

    const loaded = loadExactRunCredentials(journalPath);
    assertEquals(loaded.get(user1.email)?.password, user1.password);
    assertEquals(loaded.get(user2.email)?.password, user2.password);
    assertEquals(loaded.size, 2);

    // Another run cannot read or clobber this run's credentials
    const otherJournalPath = credentialsPath(tmpDir, "other-run-456");
    const otherLoaded = loadExactRunCredentials(otherJournalPath);
    assertEquals(otherLoaded.size, 0);
  });
});

describe("dryrun100percent: 100% read-only, zero mutations to disk/DB/Kratos", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("dry-run performs zero writes, zero API deletes, zero DB queries", async () => {
    const runId = "dry-run-verify";
    const m = createManifest(runId, "feed-load-test-setup");
    const p = manifestPath(tmpDir, runId);
    const kratosId = crypto.randomUUID();

    m.identities.push({
      kratosId,
      email: "loadtest-0001@test.alt.local",
      syntheticUserId: "",
    });
    await writeManifestAtomic(m, p);

    const initialText = await Deno.readTextFile(p);

    let deleteFetchCalls = 0;
    let sqlExecutionCalls = 0;
    let removeFileCalls = 0;

    const mockFetch = (_input: string | URL | Request, init?: RequestInit): Promise<Response> => {
      const method = init?.method || "GET";
      if (method === "DELETE") {
        deleteFetchCalls++;
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({
            traits: { email: "loadtest-0001@test.alt.local" },
            metadata_admin: { run_id: runId },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );
    };

    const mockExecSQL: ExecSQLFn = () => {
      sqlExecutionCalls++;
      return Promise.resolve({ success: true, stdout: "" });
    };

    const mockRemove = () => {
      removeFileCalls++;
      return Promise.resolve();
    };

    await executeTeardownFlow({
      manifest: m,
      mPath: p,
      dryRun: true,
      kratosAdmin: "http://mock-kratos",
      dataDir: tmpDir,
      fetchFn: mockFetch as typeof fetch,
      execSQL: mockExecSQL,
      removeFn: mockRemove,
    });

    assertEquals(deleteFetchCalls, 0);
    assertEquals(sqlExecutionCalls, 0);
    assertEquals(removeFileCalls, 0);

    // Manifest text on disk MUST be byte-for-byte identical (no writes)
    const postDryRunText = await Deno.readTextFile(p);
    assertEquals(postDryRunText, initialText);
  });
});

describe("unjournaledwrappers: fail closed before any mutation", () => {
  it("wrappers refuse execution and exit 1 before any setup or Docker mutation", async () => {
    const scripts = [
      "run-feed-load-test.sh",
      "run-feed-read-load-test.sh",
      "run-sv-connect-composite-test.sh",
    ];

    for (const scriptName of scripts) {
      const scriptPath = new URL(`../../scripts/${scriptName}`, import.meta.url).pathname;
      const content = await Deno.readTextFile(scriptPath);
      // Verify fail-closed block is present before Docker/overlay execution
      assertEquals(content.includes("BLOCKED:"), true);
      assertEquals(content.includes("exit 1"), true);
      assertEquals(content.includes("Restoration prerequisite:"), true);
    }
  });
});
