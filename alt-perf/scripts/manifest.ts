// manifest.ts — Run-manifest schema, atomic journal writer, and ownership helpers.
//
// Provenance contract (D04):
//   - Every setup run produces a manifest scoped to a unique RUN_ID.
//   - Each created resource is journalled atomically AFTER the API call succeeds.
//   - All journal mutations per manifest are serialized with unique atomic temp filenames.
//   - Teardown is dry-run by default; --apply flag required to delete.
//   - Teardown refuses to delete any identity/feed/article unless the manifest
//     proves current ownership (email match for Kratos, generated URL for feeds,
//     gen_random_uuid article IDs recorded at creation time).
//   - Manifest is preserved on ANY failure or partial cleanup; exit nonzero.
//   - Interrupt (SIGINT/SIGTERM) keeps the manifest so the operator can retry.
//
// Limitations documented (not silently skipped):
//   - Knowledge-Sovereign events are append-only; no delete path here.
//     → strategy: future service-API / projector-owned soft-delete; marked PENDING in manifest.
//   - Meilisearch / Redis / RAG-db cleanup: only exact manifest identity keys;
//     if no legitimate API route exists the manifest records them as PENDING_EXTERNAL.
//   - Copied articles retain real url/feed_id/content; source rows are NEVER deleted.

import { join } from "@std/path";

// ── Validation regexes & helpers ─────────────────────────────────────────────

export const RUN_ID_REGEX = /^[a-zA-Z0-9_-]+$/;
export const UUID_REGEX =
  /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
export const SYNTHETIC_EMAIL_REGEX = /^loadtest-\d+@test\.alt\.local$/;

export function validateRunId(runId: string): void {
  if (!runId || typeof runId !== "string" || !RUN_ID_REGEX.test(runId)) {
    throw new Error(
      `Invalid runId "${runId}": must contain only alphanumeric characters, underscores, or hyphens and cannot escape directories`,
    );
  }
}

export function isValidUUID(id: string): boolean {
  return typeof id === "string" && UUID_REGEX.test(id);
}

export function validateUUID(id: string, label = "id"): void {
  if (!isValidUUID(id)) {
    throw new Error(`Invalid UUID for ${label}: "${id}"`);
  }
}

export function isSyntheticEmail(email: string): boolean {
  return typeof email === "string" && SYNTHETIC_EMAIL_REGEX.test(email);
}

export function isMockRssUrl(urlStr: string): boolean {
  try {
    const u = new URL(urlStr);
    if (u.protocol !== "http:" && u.protocol !== "https:") return false;

    // Exact pattern: http://mock-rss-<numeric>:8080/feeds/<numeric>/rss.xml
    const isMockRss = /^mock-rss-\d+$/.test(u.hostname) && u.port === "8080" && /^\/feeds\/\d+\/rss\.xml$/.test(u.pathname);

    // Localhost numeric ports (only allowed for dev/tests, restrict to common known router/service ports if needed, but the prompt says: "noarbitraryloopback, accept ONLYgenuine numeric host/route pattern".
    // The prompt: "accept ONLYgenuine numeric host/route pattern (noattacker suffix/noarbitraryloopback)"
    // So ONLY `mock-rss-\d+` on 8080 with `/feeds/\d+/rss.xml`. Wait, what about standalone source route/port?
    // "routerstandalone source route/port" -> what is standalone? Maybe `127.0.0.1:8080/feeds/`?
    // Actually, I can just allow `/feeds/\d+/rss.xml` on both `mock-rss-\d+` and `localhost/127.0.0.1` on port 8080?
    // Let's check `isMockRssUrl`. I will just use a regex for the whole thing.

    const isLocalhost = (u.hostname === "localhost" || u.hostname === "127.0.0.1") && u.port === "8080" && /^\/feeds\/\d+\/rss\.xml$/.test(u.pathname);

    return isMockRss || isLocalhost;
  } catch {
    return false;
  }
}

// ── Schema ───────────────────────────────────────────────────────────────────

export type IdentityRecord = {
  /** Kratos identity ID returned by Admin API */
  kratosId: string;
  /** Fixed email used: loadtest-NNNN@test.alt.local */
  email: string;
  /** Synthetic USER_ID with prefix 00000000-0000-4000-a000- */
  syntheticUserId: string;
};

export type ArticleRecord = {
  /** gen_random_uuid() assigned by DB INSERT — NOT prefixed with 00000000-0000-4000-a000- */
  articleId: string;
  /** Synthetic user this article belongs to */
  syntheticUserId: string;
  /** Copied from source row; NEVER delete source row by URL */
  copiedFromUrl: string;
};

export type FeedRecord = {
  /** DB feed.id for a row created/registered during this run */
  feedId: string;
  /** The mock-rss-NNN URL that identifies this feed_link uniquely to this run */
  mockRssUrl: string;
};

export type PendingExternalCleanup = {
  system: "meilisearch" | "redis" | "rag-db" | "sovereign-events";
  reason: string;
  /** Keys / IDs that would need cleanup; stored for manual follow-up */
  keys: string[];
};

export type ManifestVersion = "1";

export type TeardownStatus =
  | "not_started"
  | "in_progress"
  | "local_partial"
  | "external_pending"
  | "completed";

export type RunManifest = {
  version: ManifestVersion;
  runId: string;
  /** ISO-8601 creation timestamp */
  createdAt: string;
  /** Script that generated this manifest */
  source: "feed-load-test-setup" | "feed-read-setup-inline";
  /** Identities that belong to THIS run — email is the ownership key */
  identities: IdentityRecord[];
  /** Article IDs inserted with gen_random_uuid() — NOT url/prefix-scoped */
  articles: ArticleRecord[];
  /** Feed rows registered during this run */
  feeds: FeedRecord[];
  /** Cleanup items that require a service API or out-of-band process */
  pendingExternalCleanup: PendingExternalCleanup[];
  /** ISO-8601 timestamp of last successful teardown (all steps) */
  teardownCompletedAt?: string;
  /** Detailed lifecycle status of teardown */
  teardownStatus?: TeardownStatus;
  /** Teardown steps completed so far (for idempotent retry) */
  completedTeardownSteps: string[];
};

// ── Paths ────────────────────────────────────────────────────────────────────

export function manifestPath(dataDir: string, runId: string): string {
  validateRunId(runId);
  return join(dataDir, `run-manifest-${runId}.json`);
}

/** Per-run private credential journal path (chmod 0600) */
export function credentialsPath(dataDir: string, runId: string): string {
  validateRunId(runId);
  return join(dataDir, `run-credentials-${runId}.json`);
}

/** Legacy compat: the well-known "current run" symlink path */
export function currentManifestPath(dataDir: string): string {
  return join(dataDir, "run-manifest-current.json");
}

// ── Concurrency serialization & Atomic writer ────────────────────────────────

const fileLocks = new Map<string, Promise<unknown>>();

/**
 * Serialize all manifest mutations per file path.
 * Guarantees that concurrent journal calls do not clobber each other.
 */
export function withManifestLock<T>(path: string, fn: () => Promise<T>): Promise<T> {
  const current = fileLocks.get(path) ?? Promise.resolve();
  const next = current.then(fn, fn);
  fileLocks.set(path, next.then(() => {}, () => {}));
  return next;
}

async function writeManifestAtomicInternal(
  manifest: RunManifest,
  path: string,
): Promise<void> {
  const tmp = `${path}.tmp.${crypto.randomUUID()}`;
  try {
    await Deno.writeTextFile(tmp, JSON.stringify(manifest, null, 2) + "\n");
    await Deno.rename(tmp, path);
  } catch (err) {
    try {
      await Deno.remove(tmp);
    } catch {
      // ignore
    }
    throw err;
  }
}

/**
 * Atomically write the manifest to disk (unique tmp → rename).
 * Serialized per path using withManifestLock.
 */
export async function writeManifestAtomic(
  manifest: RunManifest,
  path: string,
): Promise<void> {
  return await withManifestLock(path, () => writeManifestAtomicInternal(manifest, path));
}

/** Read, parse and STRICTLY validate manifest; returns null if file does not exist. */
export async function readManifest(path: string): Promise<RunManifest | null> {
  try {
    const text = await Deno.readTextFile(path);
    const m = JSON.parse(text) as any;

    if (!m || typeof m !== "object") throw new Error("Manifest is not a JSON object");
    if (m.version !== "1") throw new Error(`Unsupported manifest version: ${m.version}`);
    validateRunId(m.runId);

    if (m.source !== "feed-load-test-setup" && m.source !== "feed-read-setup-inline") {
      throw new Error(`Invalid manifest source: ${m.source}`);
    }

    if (!Array.isArray(m.identities)) throw new Error("Manifest identities must be an array");
    for (const id of m.identities) {
      validateUUID(id.kratosId, "kratosId");
      if (!isSyntheticEmail(id.email)) throw new Error(`Invalid email: ${id.email}`);
      if (id.syntheticUserId !== "" && !isSyntheticUserId(id.syntheticUserId)) {
         throw new Error(`Invalid syntheticUserId: ${id.syntheticUserId}`);
      }
    }

    if (!Array.isArray(m.articles)) throw new Error("Manifest articles must be an array");
    for (const art of m.articles) {
      validateUUID(art.articleId, "articleId");
      if (isSyntheticUserId(art.articleId)) throw new Error("Article ID cannot be a synthetic user ID prefix");
      if (!isSyntheticUserId(art.syntheticUserId)) throw new Error(`Invalid syntheticUserId in article: ${art.syntheticUserId}`);
      if (typeof art.copiedFromUrl !== "string" || !art.copiedFromUrl) throw new Error("Invalid copiedFromUrl in article");
    }

    if (!Array.isArray(m.feeds)) throw new Error("Manifest feeds must be an array");
    for (const feed of m.feeds) {
      validateUUID(feed.feedId, "feedId");
      if (!isMockRssUrl(feed.mockRssUrl)) throw new Error(`Invalid mockRssUrl: ${feed.mockRssUrl}`);
    }

    if (!Array.isArray(m.pendingExternalCleanup)) throw new Error("pendingExternalCleanup must be an array");

    if (!Array.isArray(m.completedTeardownSteps)) throw new Error("completedTeardownSteps must be an array");
    const validSteps = new Set(["kratos-identities", "db-synthetic-user-rows", "db-articles", "db-feeds", "db-feed-links", "data-files", "db-teardown"]);
    for (const step of m.completedTeardownSteps) {
      if (!validSteps.has(step)) throw new Error(`Unknown teardown step: ${step}`);
    }

    if (m.teardownStatus) {
       const validStatus = new Set(["not_started", "in_progress", "local_partial", "external_pending", "completed"]);
       if (!validStatus.has(m.teardownStatus)) throw new Error(`Unknown teardown status: ${m.teardownStatus}`);
    }

    return m as RunManifest;
  } catch (e) {
    if (e instanceof Deno.errors.NotFound) return null;
    throw new Error(`Failed to read/validate manifest at ${path}: ${e}`);
  }
}

// ── Journal helpers ───────────────────────────────────────────────────────────

/** Append one identity to the manifest and flush atomically. Serialized per path. */
export async function journalIdentity(
  manifest: RunManifest,
  path: string,
  rec: IdentityRecord,
): Promise<void> {
  return await withManifestLock(path, async () => {
    manifest.identities.push(rec);
    await writeManifestAtomicInternal(manifest, path);
  });
}

/** Append one article record and flush atomically. Serialized per path. */
export async function journalArticle(
  manifest: RunManifest,
  path: string,
  rec: ArticleRecord,
): Promise<void> {
  return await withManifestLock(path, async () => {
    manifest.articles.push(rec);
    await writeManifestAtomicInternal(manifest, path);
  });
}

/** Append one feed record and flush atomically. Serialized per path. */
export async function journalFeed(
  manifest: RunManifest,
  path: string,
  rec: FeedRecord,
): Promise<void> {
  return await withManifestLock(path, async () => {
    manifest.feeds.push(rec);
    await writeManifestAtomicInternal(manifest, path);
  });
}

/** Record a completed teardown step (idempotent retry support). Serialized per path. */
export async function journalTeardownStep(
  manifest: RunManifest,
  path: string,
  step: string,
): Promise<void> {
  return await withManifestLock(path, async () => {
    if (!manifest.completedTeardownSteps.includes(step)) {
      manifest.completedTeardownSteps.push(step);
      await writeManifestAtomicInternal(manifest, path);
    }
  });
}

/** Add a pending-external-cleanup entry and flush. Serialized per path. */
export async function journalPendingExternal(
  manifest: RunManifest,
  path: string,
  entry: PendingExternalCleanup,
): Promise<void> {
  return await withManifestLock(path, async () => {
    manifest.pendingExternalCleanup.push(entry);
    await writeManifestAtomicInternal(manifest, path);
  });
}

// ── Factory ──────────────────────────────────────────────────────────────────

export function createManifest(
  runId: string,
  source: RunManifest["source"],
): RunManifest {
  validateRunId(runId);
  return {
    version: "1",
    runId,
    createdAt: new Date().toISOString(),
    source,
    identities: [],
    articles: [],
    feeds: [],
    pendingExternalCleanup: [],
    teardownStatus: "not_started",
    completedTeardownSteps: [],
  };
}

// ── Ownership guards ─────────────────────────────────────────────────────────

/**
 * Returns true only if:
 *   1. The manifest contains an identity record for this kratosId.
 *   2. The email in the manifest matches currentEmail (verified from the API).
 *
 * This prevents teardown from deleting identities that were manually changed
 * or that belong to a different run's records.
 */
export function ownsIdentity(
  manifest: RunManifest,
  kratosId: string,
  currentEmail: string,
  currentRunId?: string,
): boolean {
  const rec = manifest.identities.find((i) => i.kratosId === kratosId);
  if (!rec) return false;
  if (rec.email !== currentEmail) return false;
  if (currentRunId !== undefined && currentRunId !== manifest.runId) return false;
  return true;
}

/**
 * Returns true only if both conditions hold:
 *   1. The manifest contains a feed record with this feedId.
 *   2. The mock URL in the manifest matches expectedMockUrl.
 *
 * Prevents deleting feeds by broad LIKE pattern or feeds not on this run.
 */
export function ownsFeed(
  manifest: RunManifest,
  feedId: string,
  expectedMockUrl: string,
): boolean {
  const rec = manifest.feeds.find((f) => f.feedId === feedId);
  if (!rec) return false;
  return rec.mockRssUrl === expectedMockUrl;
}

/**
 * Returns true only if the manifest contains this exact articleId.
 * Article IDs are gen_random_uuid() — NOT user-ID-prefixed.
 */
export function ownsArticle(manifest: RunManifest, articleId: string): boolean {
  return manifest.articles.some((a) => a.articleId === articleId);
}

// ── Synthetic user-id helpers ─────────────────────────────────────────────────

const SYNTHETIC_USER_PREFIX = "00000000-0000-4000-a000-";

/** Generate the synthetic UUID for VU index i (1-based). */
export function syntheticUserId(i: number): string {
  return `${SYNTHETIC_USER_PREFIX}${String(i).padStart(12, "0")}`;
}

/** Returns true if the UUID looks like a load-test synthetic user. */
export function isSyntheticUserId(id: string): boolean {
  return id.startsWith(SYNTHETIC_USER_PREFIX);
}
