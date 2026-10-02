#!/usr/bin/env -S deno run --allow-net --allow-read --allow-write --allow-env --allow-run
// feed-load-test-teardown.ts - Clean up load test data safely.
//
// D04 safety contract:
//   DRY-RUN IS THE DEFAULT. Pass --apply to actually delete.
//   Dry-run is 100% read-only (zero writes to disk, DB, or APIs).
//
//   Ownership proofs required before any delete:
//     Full ALL current Kratos identities preflight BEFORE ANY DB/files/external delete.
//     If any identity fails ownership proof (mismatch or unprovable error):
//       ABORT IMMEDIATELY with 0 mutations, preserving all journals and credentials.
//
//     DB records:
//       - Only exact verified articleId, feedId, and userId sets from THIS run's manifest.
//       - NEVER global prefix across runs.
//       - Shared references (feed_links used by other feeds/users) are preserved.
//       - Strict UUID validation prevents SQL injection; URL parameter escaping.
//
//   Incomplete DB/external stores retain manifest and credentials,
//   recording local_partial / external_pending, NOT completed.

import { parseArgs } from "jsr:@std/cli@^1.0.0/parse-args";
import {
  credentialsPath,
  currentManifestPath,
  type IdentityRecord,
  isMockRssUrl,
  isSyntheticEmail,
  isValidUUID,
  journalPendingExternal,
  journalTeardownStep,
  manifestPath,
  ownsArticle,
  ownsFeed,
  ownsIdentity,
  readManifest,
  type RunManifest,
  validateRunId,
  writeManifestAtomic,
} from "./manifest.ts";

const FETCH_TIMEOUT_MS = 15_000;

export function fetchOpts(init: RequestInit = {}): RequestInit {
  return { ...init, signal: AbortSignal.timeout(FETCH_TIMEOUT_MS) };
}

// ── Manifest loading ─────────────────────────────────────────────────────────

export async function loadManifest(
  runIdArg?: string,
  dataDir = new URL("../k6/data/", import.meta.url).pathname,
): Promise<{ manifest: RunManifest; mPath: string } | null> {
  let runId = runIdArg;
  let mPath: string;

  if (!runId) {
    const currentPath = currentManifestPath(dataDir);
    try {
      const ptr = JSON.parse(await Deno.readTextFile(currentPath)) as {
        runId: string;
      };
      runId = ptr.runId;
      validateRunId(runId);
      mPath = manifestPath(dataDir, runId);
    } catch (e) {
      console.error(
        `Failed to read or validate current manifest pointer at ${currentPath}: ${e}\n` +
          `Pass --run-id=<RUN_ID> explicitly.`,
      );
      return null;
    }
  } else {
    validateRunId(runId);
    mPath = manifestPath(dataDir, runId);
  }

  const manifest = await readManifest(mPath);
  if (!manifest) {
    console.error(`Manifest not found: ${mPath}`);
    return null;
  }

  if (manifest.runId !== runId) {
    console.error(`Manifest runId mismatch: expected ${runId}, got ${manifest.runId}`);
    return null;
  }

  return { manifest, mPath };
}

// ── Kratos identity preflight ────────────────────────────────────────────────

export interface PreflightResult {
  ok: boolean;
  reason?: string;
  mismatchedId?: string;
}

/**
 * Preflight check ALL current Kratos identities before any deletion.
 * Mismatch or unprovable status aborts the entire teardown before any mutations.
 */
export async function preflightKratosIdentities(
  manifest: RunManifest,
  kratosAdmin: string,
  fetchFn: typeof fetch = fetch,
): Promise<PreflightResult> {
  for (const rec of manifest.identities) {
    if (!isValidUUID(rec.kratosId)) {
      return {
        ok: false,
        reason: `Invalid UUID format for identity: ${rec.kratosId}`,
        mismatchedId: rec.kratosId,
      };
    }
    if (!isSyntheticEmail(rec.email)) {
      return {
        ok: false,
        reason: `Identity in manifest does not have synthetic test email: ${rec.email}`,
        mismatchedId: rec.kratosId,
      };
    }

    try {
      const res = await fetchFn(
        `${kratosAdmin}/admin/identities/${rec.kratosId}`,
        fetchOpts(),
      );
      if (res.status === 404) {
        // Already gone: safe, provably no collision
        continue;
      }
      if (!res.ok) {
        return {
          ok: false,
          reason: `Failed to query identity ${rec.kratosId} (HTTP ${res.status})`,
          mismatchedId: rec.kratosId,
        };
      }
      const data = (await res.json()) as {
        traits?: { email?: string };
        metadata_admin?: { run_id?: string };
      };
      const apiEmail = data.traits?.email ?? "";
      if (apiEmail !== rec.email) {
        return {
          ok: false,
          reason:
            `Ownership mismatch: identity ${rec.kratosId} email in Kratos (${apiEmail}) does not match manifest (${rec.email})`,
          mismatchedId: rec.kratosId,
        };
      }
      // Finding 2: REQUIRED exact metadata run marker; absent marker is ownership failure
      if (!data.metadata_admin?.run_id || data.metadata_admin.run_id !== manifest.runId) {
        return {
          ok: false,
          reason: `Ownership mismatch: identity ${rec.kratosId} metadata run_id (${
            data.metadata_admin?.run_id ?? "absent"
          }) does not match manifest (${manifest.runId})`,
          mismatchedId: rec.kratosId,
        };
      }
    } catch (e) {
      return {
        ok: false,
        reason: `Error connecting to Kratos for identity ${rec.kratosId}: ${e}`,
        mismatchedId: rec.kratosId,
      };
    }
  }

  return { ok: true };
}

// ── Kratos teardown ──────────────────────────────────────────────────────────

export async function teardownIdentity(
  rec: IdentityRecord,
  dryRun: boolean,
  kratosAdmin: string,
  fetchFn: typeof fetch = fetch,
  manifestRunId?: string,
): Promise<"deleted" | "skipped" | "failed"> {
  let currentEmail: string;
  let currentRunId: string | undefined;
  try {
    const res = await fetchFn(
      `${kratosAdmin}/admin/identities/${rec.kratosId}`,
      fetchOpts(),
    );
    if (res.status === 404) {
      return "deleted";
    }
    if (!res.ok) {
      console.warn(`  WARN: GET identity ${rec.kratosId} → ${res.status}; skipping`);
      return "failed";
    }
    const data = (await res.json()) as {
      traits?: { email?: string };
      metadata_admin?: { run_id?: string };
    };
    currentEmail = data.traits?.email ?? "";
    currentRunId = data.metadata_admin?.run_id;
  } catch (e) {
    console.warn(`  WARN: cannot fetch identity ${rec.kratosId}: ${e}; skipping`);
    return "failed";
  }

  const dummyManifest = {
    identities: [rec],
    runId: manifestRunId ?? "",
  } as RunManifest;

  if (!ownsIdentity(dummyManifest, rec.kratosId, currentEmail, currentRunId)) {
    console.warn(
      `  SKIP: identity ${rec.kratosId} ownership mismatch: ` +
        `manifest=${rec.email}, api=${currentEmail}, runId=${currentRunId}. Not deleting.`,
    );
    return "skipped";
  }

  if (dryRun) {
    console.log(`  [dry-run] WOULD DELETE identity ${rec.kratosId} (${rec.email})`);
    return "deleted";
  }

  try {
    const res = await fetchFn(
      `${kratosAdmin}/admin/identities/${rec.kratosId}`,
      fetchOpts({ method: "DELETE" }),
    );
    if (res.status === 204 || res.status === 200 || res.status === 404) {
      return "deleted";
    }
    console.error(`  ERROR: DELETE identity ${rec.kratosId} → ${res.status}`);
    return "failed";
  } catch (e) {
    console.error(`  ERROR: DELETE identity ${rec.kratosId}: ${e}`);
    return "failed";
  }
}

export async function teardownKratosUsers(
  manifest: RunManifest,
  mPath: string,
  dryRun: boolean,
  kratosAdmin: string,
  batchSize = 50,
  fetchFn: typeof fetch = fetch,
): Promise<boolean> {
  const STEP = "kratos-identities";
  if (manifest.completedTeardownSteps.includes(STEP)) {
    console.log(`  [skip] Kratos identities already completed in a previous run.`);
    return true;
  }

  const identities = manifest.identities;
  console.log(`Kratos identities: ${identities.length} to check (dry-run=${dryRun})`);
  let deleted = 0, skipped = 0, failed = 0;

  for (let i = 0; i < identities.length; i += batchSize) {
    const batch = identities.slice(i, i + batchSize);
    const results = await Promise.all(
      batch.map((r) => teardownIdentity(r, dryRun, kratosAdmin, fetchFn, manifest.runId)),
    );
    for (const r of results) {
      if (r === "deleted") deleted++;
      else if (r === "skipped") skipped++;
      else failed++;
    }
  }

  console.log(
    `  Kratos: ${deleted} deleted/gone, ${skipped} skipped (ownership mismatch), ${failed} failed`,
  );

  if (failed > 0 || skipped > 0) return false;

  if (!dryRun) {
    await journalTeardownStep(manifest, mPath, STEP);
  }
  return true;
}

// ── DB teardown ──────────────────────────────────────────────────────────────

export type ExecSQLFn = (
  composeParts: string[],
  label: string,
  service: string,
  user: string,
  db: string,
  sql: string,
) => Promise<{ success: boolean; stdout: string }>;

export async function defaultExecSQL(
  composeParts: string[],
  label: string,
  service: string,
  user: string,
  db: string,
  sql: string,
): Promise<{ success: boolean; stdout: string }> {
  console.log(`  Deleting ${label}...`);
  const binary = composeParts[0] || "docker";
  const cmd = new Deno.Command(binary, {
    args: [
      ...composeParts.slice(1),
      "exec",
      "-T",
      service,
      "psql",
      "-tA",
      "-v",
      "ON_ERROR_STOP=1",
      "-U",
      user,
      "-d",
      db,
      "-c",
      sql.trim(),
    ],
    stdout: "piped",
    stderr: "piped",
  });

  const output = await cmd.output();
  const stdout = new TextDecoder().decode(output.stdout);
  const stderr = new TextDecoder().decode(output.stderr);

  if (output.success) {
    console.log(`  ${label}: ${stdout.trim()}`);
    return { success: true, stdout };
  } else {
    console.error(`  ${label} failed: ${stderr}`);
    return { success: false, stdout: "" };
  }
}

/**
 * Validate and sanitize UUIDs for SQL IN clauses.
 * Throws if any UUID fails strict validation.
 */
export function buildSqlUuidInList(ids: string[]): string {
  for (const id of ids) {
    if (!isValidUUID(id)) {
      throw new Error(`Security validation failure: invalid UUID "${id}" in SQL query`);
    }
  }
  return ids.map((id) => `'${id}'`).join(",");
}

/**
 * Validate and sanitize mock URLs for SQL IN clauses.
 */
export function buildSqlMockUrlInList(urls: string[]): string {
  for (const u of urls) {
    if (!isMockRssUrl(u)) {
      throw new Error(`Security validation failure: URL is not an approved mock RSS URL "${u}"`);
    }
  }
  return urls.map((u) => `'${u.replace(/'/g, "''")}'`).join(",");
}

/**
 * Preflight CURRENT DB records: verify that articles, feeds, and subscriptions
 * provably belong to this run and do not touch foreign or shared records.
 * If any ownership check fails or cannot be proven, abort teardown with ZERO mutations.
 */
export async function preflightDatabaseOwnership(
  manifest: RunManifest,
  composeParts: string[],
  dbService: string,
  dbUser: string,
  dbName: string,
  dryRun: boolean,
  execSQL: ExecSQLFn = defaultExecSQL,
): Promise<{ ok: boolean; reason?: string }> {
  if (manifest.articles.length === 0 && manifest.feeds.length === 0) {
    return { ok: true };
  }

  const ownedUserIds = new Set(
    manifest.identities.map((i) => i.syntheticUserId || i.kratosId).filter(Boolean),
  );
  for (const art of manifest.articles) {
    if (art.syntheticUserId) ownedUserIds.add(art.syntheticUserId);
  }

  if (manifest.articles.length > 0) {
    const articleIds = manifest.articles.map((a) => a.articleId);
    try {
      const idList = buildSqlUuidInList(articleIds);
      const query =
        `SET statement_timeout = '30s'; SELECT coalesce(json_agg(json_build_object('id', id, 'user_id', user_id, 'url', url)), '[]'::json) FROM articles WHERE id IN (${idList});`;
      const res = await execSQL(
        composeParts,
        "preflight articles",
        dbService,
        dbUser,
        dbName,
        query,
      );
      if (!res.success) {
        return { ok: false, reason: "Failed to query current articles in DB during preflight." };
      }

      let rows: any[];
      try {
        const out = res.stdout.trim().split("\n").pop() || "[]";
        rows = JSON.parse(out);
      } catch (e) {
        return { ok: false, reason: `Malformed JSON returned from articles preflight: ${e}` };
      }

      if (!Array.isArray(rows)) {
        return { ok: false, reason: "Articles preflight did not return an array" };
      }

      const seenIds = new Set();
      for (const row of rows) {
        if (!row || !row.id || typeof row.id !== "string") {
          return { ok: false, reason: "Null or missing ID in article row" };
        }
        if (seenIds.has(row.id)) {
          return { ok: false, reason: `Duplicate row for article ${row.id}` };
        }
        seenIds.add(row.id);

        if (!row.user_id || !ownedUserIds.has(row.user_id)) {
          return {
            ok: false,
            reason:
              `Foreign article owner detected in DB for article ${row.id}. Expected owned user.`,
          };
        }
      }
    } catch (e) {
      return { ok: false, reason: `Article preflight check error: ${e}` };
    }
  }

  if (manifest.feeds.length > 0) {
    const feedIds = manifest.feeds.map((f) => f.feedId);
    try {
      const idList = buildSqlUuidInList(feedIds);
      const query = `SET statement_timeout = '30s';
      SELECT coalesce(json_agg(
        json_build_object(
          'id', f.id,
          'url', fl.url,
          'sub_users', (SELECT coalesce(json_agg(user_id), '[]'::json) FROM user_feed_subscriptions ufs WHERE ufs.feed_link_id = f.feed_link_id),
          'article_users', (SELECT coalesce(json_agg(a.user_id), '[]'::json) FROM articles a WHERE a.feed_id = f.id),
          'favorite_users', (SELECT coalesce(json_agg(ff.user_id), '[]'::json) FROM favorite_feeds ff WHERE ff.feed_id = f.id),
          'read_status_users', (SELECT coalesce(json_agg(rs.user_id), '[]'::json) FROM read_status rs WHERE rs.feed_id = f.id)
        )
      ), '[]'::json)
      FROM feeds f
      JOIN feed_links fl ON f.feed_link_id = fl.id
      WHERE f.id IN (${idList});`;

      const res = await execSQL(composeParts, "preflight feeds", dbService, dbUser, dbName, query);
      if (!res.success) {
        return { ok: false, reason: "Failed to query current feeds in DB during preflight." };
      }

      let rows: any[];
      try {
        const out = res.stdout.trim().split("\n").pop() || "[]";
        rows = JSON.parse(out);
      } catch (e) {
        return { ok: false, reason: `Malformed JSON returned from feeds preflight: ${e}` };
      }

      if (!Array.isArray(rows)) {
        return { ok: false, reason: "Feeds preflight did not return an array" };
      }

      const seenIds = new Set();
      for (const row of rows) {
        if (!row || !row.id || typeof row.id !== "string") {
          return { ok: false, reason: "Null or missing ID in feed row" };
        }
        if (seenIds.has(row.id)) return { ok: false, reason: `Duplicate row for feed ${row.id}` };
        seenIds.add(row.id);

        const manifestFeed = manifest.feeds.find((f) => f.feedId === row.id);
        if (manifestFeed && row.url !== manifestFeed.mockRssUrl) {
          return {
            ok: false,
            reason:
              `Feed URL mismatch or foreign feed link in DB for feed ${row.id}. Expected ${manifestFeed.mockRssUrl}, got ${row.url}`,
          };
        }

        const checkForeignUsers = (users: any, label: string) => {
          if (Array.isArray(users)) {
            for (const uid of users) {
              if (uid && !ownedUserIds.has(uid)) {
                return {
                  ok: false,
                  reason: `Foreign ${label} dependency detected on feed ${row.id} by user ${uid}`,
                };
              }
            }
          }
          return null;
        };

        const subErr = checkForeignUsers(row.sub_users, "shared subscription");
        if (subErr) return subErr;
        const artErr = checkForeignUsers(row.article_users, "article");
        if (artErr) return artErr;
        const favErr = checkForeignUsers(row.favorite_users, "favorite_feeds");
        if (favErr) return favErr;
        const rsErr = checkForeignUsers(row.read_status_users, "read_status");
        if (rsErr) return rsErr;
      }
    } catch (e) {
      return { ok: false, reason: `Feed preflight check error: ${e}` };
    }
  }

  return { ok: true };
}

export async function teardownDatabaseAtomic(
  manifest: RunManifest,
  mPath: string,
  composeParts: string[],
  dbService: string,
  dbUser: string,
  dbName: string,
  dryRun: boolean,
  execSQL: ExecSQLFn = defaultExecSQL,
): Promise<boolean> {
  const STEP_DB = "db-teardown";

  const ownedUserIdSet = new Set<string>();
  for (const r of manifest.identities) {
    const id = r.syntheticUserId || r.kratosId;
    if (id && isValidUUID(id)) {
      ownedUserIdSet.add(id);
    }
  }
  for (const art of manifest.articles) {
    if (art.syntheticUserId && isValidUUID(art.syntheticUserId)) {
      ownedUserIdSet.add(art.syntheticUserId);
    }
  }
  const ownedUserIds = Array.from(ownedUserIdSet);

  const rawArticleIds = manifest.articles
    .filter((a) => ownsArticle(manifest, a.articleId))
    .map((a) => a.articleId);

  const rawFeedIds = manifest.feeds
    .filter((f) => ownsFeed(manifest, f.feedId, f.mockRssUrl))
    .map((f) => f.feedId);

  const mockUrls = manifest.feeds
    .filter((f) => ownsFeed(manifest, f.feedId, f.mockRssUrl))
    .map((f) => f.mockRssUrl);

  if (ownedUserIds.length === 0 && rawArticleIds.length === 0 && rawFeedIds.length === 0) {
    console.log("  No DB records to teardown.");
    return true;
  }

  const userIdsSql = ownedUserIds.length > 0
    ? buildSqlUuidInList(ownedUserIds)
    : "SELECT NULL::uuid WHERE FALSE";
  const articleIdsSql = rawArticleIds.length > 0
    ? buildSqlUuidInList(rawArticleIds)
    : "SELECT NULL::uuid WHERE FALSE";
  const feedIdsSql = rawFeedIds.length > 0
    ? buildSqlUuidInList(rawFeedIds)
    : "SELECT NULL::uuid WHERE FALSE";
  const urlsSql = mockUrls.length > 0
    ? buildSqlMockUrlInList(mockUrls)
    : "SELECT NULL::text WHERE FALSE";

  // One session, BEGIN/COMMIT bounding, strict locks blocking FK/inserts
  // Re-run all preflights internally under lock, RAISE EXCEPTION on failure
  // Reconcile stale completed steps by ALWAYS querying/deleting (DELETE is idempotent).
  const sql = `
BEGIN;
SET LOCAL statement_timeout = '300s';
SET LOCAL lock_timeout = '10s';

LOCK TABLE feed_links, feeds, user_feed_subscriptions, favorite_feeds, read_status, articles, article_heads, article_summaries IN SHARE ROW EXCLUSIVE MODE;

DO $$
DECLARE
    bad_count INT;
BEGIN
    SELECT count(*) INTO bad_count
    FROM articles a
    WHERE a.id IN (${articleIdsSql})
      AND a.user_id NOT IN (${userIdsSql});
    IF bad_count > 0 THEN
        RAISE EXCEPTION 'Ownership preflight failed: Foreign article owner detected in DB';
    END IF;

    SELECT count(*) INTO bad_count
    FROM feeds f
    JOIN feed_links fl ON f.feed_link_id = fl.id
    WHERE f.id IN (${feedIdsSql})
      AND fl.url NOT IN (${urlsSql});
    IF bad_count > 0 THEN
        RAISE EXCEPTION 'Ownership preflight failed: Feed URL mismatch or foreign feed link in DB';
    END IF;

    SELECT count(*) INTO bad_count
    FROM user_feed_subscriptions ufs
    JOIN feed_links fl ON ufs.feed_link_id = fl.id
    WHERE fl.url IN (${urlsSql})
      AND ufs.user_id NOT IN (${userIdsSql});
    IF bad_count > 0 THEN
        RAISE EXCEPTION 'Ownership preflight failed: Foreign shared subscription dependency detected';
    END IF;

    SELECT count(*) INTO bad_count
    FROM favorite_feeds ff
    WHERE ff.feed_id IN (${feedIdsSql})
      AND ff.user_id NOT IN (${userIdsSql});
    IF bad_count > 0 THEN
        RAISE EXCEPTION 'Ownership preflight failed: Foreign favorite_feeds dependency detected';
    END IF;

    SELECT count(*) INTO bad_count
    FROM read_status rs
    WHERE rs.feed_id IN (${feedIdsSql})
      AND rs.user_id NOT IN (${userIdsSql});
    IF bad_count > 0 THEN
        RAISE EXCEPTION 'Ownership preflight failed: Foreign read_status dependency detected';
    END IF;

    SELECT count(*) INTO bad_count
    FROM articles a
    WHERE a.feed_id IN (${feedIdsSql})
      AND a.user_id NOT IN (${userIdsSql});
    IF bad_count > 0 THEN
        RAISE EXCEPTION 'Ownership preflight failed: Foreign article dependency detected on feed';
    END IF;

    -- Deletions
    DELETE FROM article_summaries WHERE article_id IN (${articleIdsSql});
    DELETE FROM article_heads WHERE article_id IN (${articleIdsSql});
    DELETE FROM articles WHERE id IN (${articleIdsSql});

    DELETE FROM favorite_feeds WHERE feed_id IN (${feedIdsSql});
    DELETE FROM read_status WHERE feed_id IN (${feedIdsSql});
    DELETE FROM user_feed_subscriptions WHERE user_id IN (${userIdsSql});
    DELETE FROM feeds WHERE id IN (${feedIdsSql});

    DELETE FROM feed_links fl
    WHERE fl.url IN (${urlsSql})
      AND NOT EXISTS (
        SELECT 1 FROM feeds f
        WHERE f.feed_link_id = fl.id
          AND f.id NOT IN (${feedIdsSql})
      );

    -- Post-delete verification
    SELECT count(*) INTO bad_count FROM articles WHERE id IN (${articleIdsSql});
    IF bad_count > 0 THEN RAISE EXCEPTION 'Rollback: target articles not absent after delete'; END IF;

    SELECT count(*) INTO bad_count FROM feeds WHERE id IN (${feedIdsSql});
    IF bad_count > 0 THEN RAISE EXCEPTION 'Rollback: target feeds not absent after delete'; END IF;

    SELECT count(*) INTO bad_count FROM user_feed_subscriptions WHERE user_id IN (${userIdsSql});
    IF bad_count > 0 THEN RAISE EXCEPTION 'Rollback: target subscriptions not absent after delete'; END IF;

END $$;

COMMIT;
`;

  if (dryRun) {
    console.log(
      `  [dry-run] WOULD EXECUTE atomic DB teardown transaction for ${rawArticleIds.length} articles, ${rawFeedIds.length} feeds, ${ownedUserIds.length} users.`,
    );
    return true;
  }

  const r = await execSQL(composeParts, "atomic DB teardown", dbService, dbUser, dbName, sql);
  if (!r.success) return false;

  await journalTeardownStep(manifest, mPath, STEP_DB);
  return true;
}

// ── External systems ─────────────────────────────────────────────────────────

export async function reportPendingExternal(
  manifest: RunManifest,
  mPath: string,
  dryRun: boolean,
): Promise<void> {
  console.log("  External systems (Meili / Redis / RAG / Sovereign):");
  const systems = manifest.pendingExternalCleanup.map((e) => e.system);
  const toAdd: Array<typeof manifest.pendingExternalCleanup[0]> = [];

  if (!systems.includes("meilisearch")) {
    toAdd.push({
      system: "meilisearch",
      reason: "No exact manifest route for Meilisearch doc cleanup. " +
        "Run IDs: see manifest.articles[].articleId. " +
        "Use Meilisearch Admin API deleteDocuments with these IDs directly. PENDING_EXTERNAL.",
      keys: manifest.articles.map((a) => a.articleId),
    });
  }
  if (!systems.includes("redis")) {
    toAdd.push({
      system: "redis",
      reason: "Redis session/cache entries for synthetic users expire naturally (TTL). " +
        "No broad flush needed. PENDING_EXTERNAL for manual verification.",
      keys: manifest.identities.map((i) => i.kratosId),
    });
  }

  for (const entry of toAdd) {
    if (dryRun) {
      console.log(`  [dry-run] WOULD NOTE PENDING_EXTERNAL: ${entry.system}`);
    } else {
      await journalPendingExternal(manifest, mPath, entry);
      console.log(`  Noted PENDING_EXTERNAL: ${entry.system}`);
    }
  }

  for (const entry of manifest.pendingExternalCleanup) {
    console.log(`    [${entry.system}] ${entry.reason}`);
  }
}

// ── Data files ───────────────────────────────────────────────────────────────

export async function removeDataFiles(
  manifest: RunManifest,
  mPath: string,
  dryRun: boolean,
  dataDir = new URL("../k6/data/", import.meta.url).pathname,
  removeFn: (path: string) => Promise<void> = (p) => Deno.remove(p),
): Promise<void> {
  const STEP = "data-files";
  if (manifest.completedTeardownSteps.includes(STEP)) {
    console.log(`  [skip] Data files already removed.`);
    return;
  }

  // 1. Per-run credential journal: exact run artifact
  const runCredPath = credentialsPath(dataDir, manifest.runId);
  if (dryRun) {
    console.log(`  [dry-run] WOULD REMOVE ${runCredPath}`);
  } else {
    try {
      await removeFn(runCredPath);
      console.log(`  Removed ${runCredPath}`);
    } catch {
      // not found
    }
  }

  // 2. Shared files: ONLY remove if provably owned by this run
  let isCurrentRun = false;
  try {
    const currentPath = currentManifestPath(dataDir);
    const ptr = JSON.parse(await Deno.readTextFile(currentPath)) as { runId: string };
    if (ptr.runId === manifest.runId) {
      isCurrentRun = true;
    }
  } catch {
    // pointer not found
  }

  if (isCurrentRun) {
    for (const file of ["load-test-users.json", "load-test-users.csv"]) {
      const path = `${dataDir}/${file}`;
      if (dryRun) {
        console.log(`  [dry-run] WOULD REMOVE ${path}`);
      } else {
        try {
          await removeFn(path);
          console.log(`  Removed ${path}`);
        } catch {
          // not found
        }
      }
    }
  } else {
    console.log(`  Preserving shared files that do not provably belong to run ${manifest.runId}`);
  }

  if (!dryRun) {
    await journalTeardownStep(manifest, mPath, STEP);
  }
}

// ── Orchestrator ─────────────────────────────────────────────────────────────

export interface TeardownOptions {
  manifest: RunManifest;
  mPath: string;
  dryRun: boolean;
  kratosAdmin: string;
  dataDir?: string;
  composeParts?: string[];
  dbService?: string;
  dbUser?: string;
  dbName?: string;
  fetchFn?: typeof fetch;
  execSQL?: ExecSQLFn;
  removeFn?: (path: string) => Promise<void>;
}

export async function executeTeardownFlow(
  opts: TeardownOptions,
): Promise<{ success: boolean; status: RunManifest["teardownStatus"] }> {
  const {
    manifest,
    mPath,
    dryRun,
    kratosAdmin,
    dataDir = new URL("../k6/data/", import.meta.url).pathname,
    composeParts = ["docker", "compose", "-f", "compose/compose.yaml", "-p", "alt"],
    dbService = "db",
    dbUser = "alt_db_user",
    dbName = "alt",
    fetchFn = fetch,
    execSQL = defaultExecSQL,
    removeFn = (p) => Deno.remove(p),
  } = opts;

  // PREFLIGHT 1: Check ALL Kratos identities BEFORE any mutation
  console.log("--- Preflight: Validating Kratos identities ownership ---");
  const preflight = await preflightKratosIdentities(manifest, kratosAdmin, fetchFn);
  if (!preflight.ok) {
    console.error(
      `\nABORT: Kratos identity preflight failed: ${preflight.reason}\n` +
        `Zero mutations performed. All journals and credentials preserved.`,
    );
    return { success: false, status: "local_partial" };
  }
  console.log("  Preflight passed: all identities provably belong to this run.");

  // PREFLIGHT 2: Check DB records ownership BEFORE any delete
  console.log("--- Preflight: Validating DB records ownership ---");
  const dbPreflight = await preflightDatabaseOwnership(
    manifest,
    composeParts,
    dbService,
    dbUser,
    dbName,
    dryRun,
    execSQL,
  );
  if (!dbPreflight.ok) {
    console.error(
      `\nABORT: DB records ownership preflight failed: ${dbPreflight.reason}\n` +
        `Zero mutations performed. All journals and credentials preserved.`,
    );
    return { success: false, status: "local_partial" };
  }
  console.log("  DB Preflight passed: all DB records provably belong to this run.");

  let anyFailure = false;

  // Step 1: Kratos identities
  console.log("\n--- Step 1: Kratos identities ---");
  const kratosOk = await teardownKratosUsers(manifest, mPath, dryRun, kratosAdmin, 50, fetchFn);
  if (!kratosOk) {
    console.error(
      "  Kratos teardown had failures. Stopping all subsequent SQL and dependent deletions.",
    );
    if (!dryRun) {
      manifest.teardownStatus = "local_partial";
      await writeManifestAtomic(manifest, mPath);
    }
    return { success: false, status: "local_partial" };
  }

  // Step 2 & 3: Atomic DB teardown (re-runs preflight with locks + exact deletes)
  console.log("\n--- Step 2 & 3: Atomic DB teardown ---");
  const dbOk = await teardownDatabaseAtomic(
    manifest,
    mPath,
    composeParts,
    dbService,
    dbUser,
    dbName,
    dryRun,
    execSQL,
  );
  if (!dbOk) {
    console.error("  DB teardown had failures.");
    anyFailure = true;
  }

  // Step 4: External systems report
  console.log("\n--- Step 4: External systems report ---");
  await reportPendingExternal(manifest, mPath, dryRun);

  if (anyFailure) {
    console.warn("\n--- Preserving data and credential files due to prior failures ---");
    if (!dryRun) {
      manifest.teardownStatus = "local_partial";
      await writeManifestAtomic(manifest, mPath);
    }
    return { success: false, status: "local_partial" };
  }

  // Reviewer Finding 4: credentials currently deleted BEFORE external_pending.
  // Keep per-run credentials/manifest on ANY partial failure/pending store.
  const hasExternalPending = manifest.pendingExternalCleanup.length > 0;
  if (hasExternalPending) {
    console.warn(
      "\n--- Preserving credentials and manifest due to pending external cleanup items ---",
    );
    if (!dryRun) {
      manifest.teardownStatus = "external_pending";
      await writeManifestAtomic(manifest, mPath);
    }
    return { success: true, status: "external_pending" };
  }

  // Step 5: Data files & per-run credentials (ONLY if no local failures AND no external pending items)
  console.log("\n--- Step 5: Data and credential files cleanup ---");
  await removeDataFiles(manifest, mPath, dryRun, dataDir, removeFn);

  if (!dryRun) {
    manifest.teardownStatus = "completed";
    manifest.teardownCompletedAt = new Date().toISOString();
    await writeManifestAtomic(manifest, mPath);
  }
  return { success: true, status: "completed" };
}

// ── Main ──────────────────────────────────────────────────────────────────────

async function main() {
  const args = parseArgs(Deno.args, {
    boolean: ["apply", "help"],
    string: ["run-id"],
    default: { apply: false, "run-id": "" },
  });

  if (args.help) {
    console.log(`Usage:
  feed-load-test-teardown.ts [--run-id=<id>] [--apply]

  --run-id   Run ID to clean up. If omitted, reads from run-manifest-current.json.
  --apply    Actually delete resources. Default: dry-run only (100% read-only).
`);
    Deno.exit(0);
  }

  const DRY_RUN = !args.apply;
  const KRATOS_ADMIN = Deno.env.get("KRATOS_ADMIN_URL") || "http://localhost:4434";
  const COMPOSE_CMD = Deno.env.get("COMPOSE_CMD") ||
    "docker compose -f compose/compose.yaml -p alt";
  const composeParts = COMPOSE_CMD.split(" ");
  const dbService = Deno.env.get("DB_SERVICE") || "db";
  const dbUser = Deno.env.get("POSTGRES_USER") || "alt_db_user";
  const dbName = Deno.env.get("POSTGRES_DB") || "alt";

  console.log("=== Feed Load Test Teardown ===");
  console.log(`Mode: ${DRY_RUN ? "DRY-RUN (pass --apply to delete)" : "APPLY"}\n`);

  const rawRunId = args["run-id"] as string;
  const loaded = await loadManifest(rawRunId);
  if (!loaded) {
    Deno.exit(1);
  }
  const { manifest, mPath } = loaded;

  console.log(`Run ID:     ${manifest.runId}`);
  console.log(`Created:    ${manifest.createdAt}`);
  console.log(`Identities: ${manifest.identities.length}`);
  console.log(`Articles:   ${manifest.articles.length}`);
  console.log(`Feeds:      ${manifest.feeds.length}`);
  console.log(`Completed steps: ${manifest.completedTeardownSteps.join(", ") || "(none)"}\n`);

  const result = await executeTeardownFlow({
    manifest,
    mPath,
    dryRun: DRY_RUN,
    kratosAdmin: KRATOS_ADMIN,
    composeParts,
    dbService,
    dbUser,
    dbName,
  });

  if (!result.success) {
    console.error(`\nTeardown halted with status: ${result.status}. Manifest preserved: ${mPath}`);
    Deno.exit(1);
  }

  if (result.status === "external_pending") {
    console.log(
      `\nTeardown finished with external cleanup items pending. Status: external_pending.`,
    );
    console.log(`Manifest preserved for audit: ${mPath}`);
  } else if (!DRY_RUN) {
    console.log(`\nTeardown fully complete. Manifest marked completed: ${mPath}`);
  } else {
    console.log(`\nDry-run complete. Zero external mutations performed.`);
  }
}

if (import.meta.main) {
  main();
}
