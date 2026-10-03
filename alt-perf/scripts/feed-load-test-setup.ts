#!/usr/bin/env -S deno run --allow-net --allow-write --allow-read --allow-env
// feed-load-test-setup.ts - Create test users in Kratos for feed registration load test
//
// Creates N users via Kratos Admin API with email/password credentials.
// Outputs user list as JSON + CSV for K6 SharedArray consumption.
// Writes a per-run manifest (run-manifest-<RUN_ID>.json) with incremental atomic
// journal entries after each successful create — so an interrupted setup leaves
// a recoverable manifest listing exactly which identities were created.
//
// D04 safety rules:
//   - 409 Conflict: only deletes the existing identity if the manifest proves this
//     run already created it (email + kratosId match). If the identity belongs to
//     an unknown/prior run, setup FAILS with a clear error rather than blindly
//     deleting another run's user.
//   - Every created identity is journalled before moving to the next batch item.
//   - Manifest is kept on any failure; exit nonzero.
//
// Usage:
//   deno run --allow-net --allow-write --allow-read --allow-env \
//     alt-perf/scripts/feed-load-test-setup.ts
//   deno run ... feed-load-test-setup.ts --count=10
//   deno run ... feed-load-test-setup.ts --run-id=my-run-001  # explicit run ID

import { parseArgs } from "jsr:@std/cli@^1.0.0/parse-args";
import {
  createManifest,
  credentialsPath,
  currentManifestPath,
  type IdentityRecord,
  isSyntheticEmail,
  journalIdentity,
  journalPendingExternal,
  manifestPath,
  type RunManifest,
  validateRunId,
  withManifestLock,
  writeManifestAtomic,
  readManifest,
} from "./manifest.ts";

export interface TestUser {
  email: string;
  password: string;
  userId: string;
  intentNonce?: string;
}

const FETCH_TIMEOUT_MS = 15_000;

export function fetchOpts(init: RequestInit = {}): RequestInit {
  return {
    ...init,
    signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
  };
}

/**
 * Generate a cryptographically random, high-entropy test password.
 * Uses crypto.getRandomValues to prevent predictable passwords.
 */
export function generateCryptoRandomPassword(): string {
  const envPass = Deno.env.get("TEST_USER_PASSWORD");
  if (envPass) return envPass;
  const randBytes = new Uint8Array(24);
  crypto.getRandomValues(randBytes);
  const base = Array.from(randBytes, (b) => b.toString(16).padStart(2, "0")).join("");
  return `AltPass_!A1${base}`;
}

/**
 * Backward compatibility alias: now calls generateCryptoRandomPassword.
 * Deterministic generation is strictly prohibited.
 */
export function getDeterministicPassword(_runId?: string, _email?: string): string {
  return generateCryptoRandomPassword();
}

/**
 * Incrementally persist a user credential to the per-run private credential journal with chmod 0600.
 * Serialized per path using withManifestLock.
 */
export async function persistCredentialIncremental(
  journalPath: string,
  user: TestUser,
): Promise<void> {
  await withManifestLock(journalPath, async () => {
    let list: TestUser[] = [];
    try {
      const text = await Deno.readTextFile(journalPath);
      list = JSON.parse(text) as TestUser[];
    } catch {
      // not yet created or empty
    }
    const idx = list.findIndex((u) => u.email === user.email);
    if (idx >= 0) {
      list[idx] = user;
    } else {
      list.push(user);
    }
    const tmp = `${journalPath}.tmp.${crypto.randomUUID()}`;
    await Deno.writeTextFile(tmp, JSON.stringify(list, null, 2) + "\n");
    try {
      await Deno.chmod(tmp, 0o600);
    } catch {
      // ignore on unsupported OS
    }
    await Deno.rename(tmp, journalPath);
    try {
      await Deno.chmod(journalPath, 0o600);
    } catch {
      // ignore
    }
  });
}

/**
 * Load existing credentials from the exact per-run private credential journal.
 * Refuses to load from foreign or generic files; ensures 0600 private scope.
 */
export function loadExactRunCredentials(journalPath: string): Map<string, TestUser> {
  const creds = new Map<string, TestUser>();
  try {
    const text = Deno.readTextFileSync(journalPath);
    const list = JSON.parse(text) as TestUser[];
    for (const u of list) {
      if (u.email && u.password && !u.password.includes("not-available")) {
        creds.set(u.email, u);
      }
    }
  } catch {
    // file does not exist or invalid
  }
  return creds;
}

/** Legacy alias */
export function loadExistingCredentials(jsonPath: string): Map<string, string> {
  const exact = loadExactRunCredentials(jsonPath);
  const m = new Map<string, string>();
  for (const [k, v] of exact.entries()) {
    m.set(k, v.password);
  }
  return m;
}

/** Look up the first identity matching this email via Kratos Admin API. */
export async function lookupIdentityByEmail(
  email: string,
  kratosAdmin: string,
  fetchFn: typeof fetch = fetch,
): Promise<{ id: string; emailFromApi: string; runIdMarker?: string; intentNonce?: string } | null> {
  const listRes = await fetchFn(
    `${kratosAdmin}/admin/identities?credentials_identifier=${encodeURIComponent(email)}`,
    fetchOpts(),
  );
  if (!listRes.ok) return null;
  const identities = (await listRes.json()) as Array<{
    id?: string;
    traits?: { email?: string };
    metadata_admin?: { run_id?: string; intent_nonce?: string };
  }>;
  if (identities.length === 0) return null;
  const first = identities[0];
  return {
    id: first?.id ?? "",
    emailFromApi: first?.traits?.email ?? "",
    runIdMarker: first?.metadata_admin?.run_id,
    intentNonce: first?.metadata_admin?.intent_nonce,
  };
}

/**
 * Create one user. If 409, only delete+recreate if manifest proves THIS run owns it.
 * Returns null (and logs error) if ownership cannot be proven.
 */
export async function createUser(
  index: number,
  manifest: RunManifest,
  mPath: string,
  kratosAdmin: string,
  retries = 1,
  fetchFn: typeof fetch = fetch,
  cJournalPath?: string,
): Promise<TestUser | null> {
  const email = `loadtest-${String(index).padStart(4, "0")}@test.alt.local`;

  let password = "";
  let intentNonce = "";
  let intentNewlyGenerated = false;
  if (cJournalPath) {
    const saved = loadExactRunCredentials(cJournalPath);
    if (saved.has(email)) {
      const u = saved.get(email)!;
      password = u.password;
      intentNonce = u.intentNonce || "";
    } else {
      password = generateCryptoRandomPassword();
      intentNonce = crypto.randomUUID();
      intentNewlyGenerated = true;
      await persistCredentialIncremental(cJournalPath, { email, password, userId: "", intentNonce });
    }
  } else {
    password = generateCryptoRandomPassword();
    intentNonce = crypto.randomUUID();
    intentNewlyGenerated = true;
  }

  const body = {
    schema_id: "default",
    traits: { email },
    metadata_admin: {
      run_id: manifest.runId,
      marker: "alt-perf-load-test",
      intent_nonce: intentNonce,
    },
    credentials: { password: { config: { password } } },
    state: "active",
  };

  for (let attempt = 0; attempt <= retries; attempt++) {
    try {
      const res = await fetchFn(
        `${kratosAdmin}/admin/identities`,
        fetchOpts({
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        }),
      );

      if (res.status === 201 || res.status === 200) {
        const data = (await res.json()) as { id: string };
        const testUser: TestUser = { email, password, userId: data.id, intentNonce };
        if (cJournalPath) {
          await persistCredentialIncremental(cJournalPath, testUser);
        }
        const rec: IdentityRecord = {
          kratosId: data.id,
          email,
          syntheticUserId: "",
        };
        await journalIdentity(manifest, mPath, rec);
        return testUser;
      }

      if (res.status === 409) {
        await res.body?.cancel();
        console.warn(`409: identity ${email} already exists — checking ownership...`);

        const existing = await lookupIdentityByEmail(email, kratosAdmin, fetchFn);
        if (!existing || !existing.id) {
          console.error(`  FAIL: 409 but could not look up existing identity for ${email}. Skipping.`);
          return null;
        }

        const ownedByThisRun = existing.runIdMarker === manifest.runId && isSyntheticEmail(existing.emailFromApi);
        if (!ownedByThisRun) {
          console.error(
            `  FAIL: identity ${email} exists but metadata run_id does not match this run. Refusing to use or delete unknown identity.`
          );
          return null;
        }

        // Exact nonce match
        if (existing.intentNonce !== intentNonce) {
           console.error(`  FAIL: identity ${email} owned by this run, but intent_nonce mismatch. Corrupted intent fail closed.`);
           return null;
        }

        if (cJournalPath) {
          if (intentNewlyGenerated) {
             console.error(`  FAIL: identity ${email} owned by this run, but missing from credentials intent prior to this attempt. Corrupted intent fail closed.`);
             return null;
          }
          const saved = loadExactRunCredentials(cJournalPath);
          if (saved.has(email)) {
            console.warn(`  Recovered owned identity ${email} (id=${existing.id}) using prior creation intent.`);
            const testUser: TestUser = { email, password: saved.get(email)!.password, userId: existing.id, intentNonce };
            await persistCredentialIncremental(cJournalPath, testUser);
            const rec: IdentityRecord = {
              kratosId: existing.id,
              email,
              syntheticUserId: "",
            };
            await journalIdentity(manifest, mPath, rec);
            return testUser;
          } else {
             console.error(`  FAIL: identity ${email} owned by this run, but missing from credentials intent. Corrupted intent fail closed.`);
             return null;
          }
        }

        console.error(`  FAIL: identity ${email} owned by this run, but no credential journal path provided.`);
        return null;
      }

      const errBody = await res.text();
      console.error(`Failed to create ${email}: status=${res.status} body=${errBody}`);

      if (attempt < retries) {
        await new Promise((r) => setTimeout(r, 1000));
      }
    } catch (e) {
      console.error(`Error creating ${email}: ${e}`);
      if (attempt < retries) {
        await new Promise((r) => setTimeout(r, 1000));
      }
    }
  }
  return null;
}

export async function runSetup(opts: {
  userCount: number;
  runId: string;
  outputDir: string;
  kratosAdmin: string;
  batchSize?: number;
  fetchFn?: typeof fetch;
}): Promise<{ manifest: RunManifest; users: TestUser[]; failed: number }> {
  validateRunId(opts.runId);
  const { userCount, runId, outputDir, kratosAdmin } = opts;
  const batchSize = opts.batchSize ?? 50;
  const fetchFn = opts.fetchFn ?? fetch;

  await Deno.mkdir(outputDir, { recursive: true });
  const mPath = manifestPath(outputDir, runId);
  const cJournalPath = credentialsPath(outputDir, runId);

  let manifest: RunManifest;
  const existing = await readManifest(mPath);

  if (existing) {
    manifest = existing;
    console.log(
      `Resuming existing manifest (${manifest.identities.length} identities already recorded).`,
    );
  } else {
    manifest = createManifest(runId, "feed-load-test-setup");
    await writeManifestAtomic(manifest, mPath);
    console.log(`Created new manifest: ${mPath}`);
  }

  // Mark Sovereign events as unmanaged
  await journalPendingExternal(manifest, mPath, {
    system: "sovereign-events",
    reason: "Knowledge-Sovereign event log is append-only. " +
      "No delete path in setup/teardown. " +
      "Future strategy: service-API soft-delete / projector-owned data. PENDING.",
    keys: [],
  });

  const users: TestUser[] = [];
  let failed = 0;

  for (let batchStart = 0; batchStart < userCount; batchStart += batchSize) {
    const batchEnd = Math.min(batchStart + batchSize, userCount);
    const promises: Promise<TestUser | null>[] = [];

    for (let i = batchStart; i < batchEnd; i++) {
      const email = `loadtest-${String(i).padStart(4, "0")}@test.alt.local`;
      if (manifest.identities.some((r) => r.email === email)) {
        continue;
      }
      promises.push(createUser(i, manifest, mPath, kratosAdmin, 1, fetchFn, cJournalPath));
    }

    const results = await Promise.all(promises);
    for (const result of results) {
      if (result) {
        users.push(result);
      } else {
        failed++;
      }
    }

    if ((batchStart + batchSize) % 100 === 0 || batchEnd === userCount) {
      console.log(
        `Progress: ${manifest.identities.length}/${userCount} journalled, ${failed} failed`,
      );
    }
  }

  // Rebuild users list from manifest + exact run credential journal
  const savedCreds = loadExactRunCredentials(cJournalPath);
  const allUsers: TestUser[] = manifest.identities
    .filter((r) => r.syntheticUserId === "")
    .map((r) => {
      const freshUser = users.find((u) => u.userId === r.kratosId);
      if (freshUser) return freshUser;
      const pwd = savedCreds.get(r.email);
      if (!pwd) {
        throw new Error(
          `Cannot resume user ${r.email}: password not found in exact run credential journal (${cJournalPath}). Deterministic passwords are prohibited.`,
        );
      }
      return { email: r.email, password: pwd.password, userId: r.kratosId };
    });

  // Failsafe check: reject any placeholder or empty password
  for (const u of allUsers) {
    if (!u.password || u.password.includes("not-available")) {
      throw new Error(
        `Failsafe: Cannot recover valid password for ${u.email}. Aborting credential output.`,
      );
    }
  }

  // Write JSON
  const jsonPath = `${outputDir}/load-test-users.json`;
  await Deno.writeTextFile(jsonPath, JSON.stringify(allUsers, null, 2));
  try {
    await Deno.chmod(jsonPath, 0o600);
  } catch {
    // ignore chmod failure on unsupported OS
  }

  // Write CSV
  const csvPath = `${outputDir}/load-test-users.csv`;
  const csvLines = ["email,password,userId"];
  for (const u of allUsers) {
    csvLines.push(`${u.email},${u.password},${u.userId}`);
  }
  await Deno.writeTextFile(csvPath, csvLines.join("\n") + "\n");
  try {
    await Deno.chmod(csvPath, 0o600);
  } catch {
    // ignore
  }

  // Write "current" pointer
  const currentPath = currentManifestPath(outputDir);
  await Deno.writeTextFile(currentPath, JSON.stringify({ runId, manifestPath: mPath }));

  return { manifest, users: allUsers, failed };
}

async function main() {
  const args = parseArgs(Deno.args, {
    default: { count: 1000, "run-id": "" },
    alias: { n: "count" },
    string: ["run-id"],
  });

  const USER_COUNT = Number(args.count);
  const KRATOS_ADMIN = Deno.env.get("KRATOS_ADMIN_URL") || "http://localhost:4434";
  const OUTPUT_DIR = new URL("../k6/data/", import.meta.url).pathname;

  const rawRunId = (args["run-id"] as string) || "";
  let runId = rawRunId;
  if (!runId) {
    const now = new Date();
    runId = `${now.toISOString().replace(/[:.]/g, "-").slice(0, 19)}_${
      crypto.randomUUID().slice(0, 8)
    }`;
  }
  validateRunId(runId);

  console.log(`Creating ${USER_COUNT} test users via ${KRATOS_ADMIN}...`);
  console.log(`Run ID: ${runId}`);

  try {
    const { manifest, failed } = await runSetup({
      userCount: USER_COUNT,
      runId,
      outputDir: OUTPUT_DIR,
      kratosAdmin: KRATOS_ADMIN,
    });

    console.log(
      `\nSetup complete: ${manifest.identities.length} identities journalled, ${failed} failed`,
    );
    console.log(`Manifest: ${manifestPath(OUTPUT_DIR, runId)}`);

    if (failed > 0) {
      console.error(
        `\nSetup had ${failed} failure(s). Manifest preserved. Re-run with --run-id=${runId} to resume.`,
      );
      Deno.exit(1);
    }
  } catch (err) {
    console.error(`\nFatal error in setup: ${err}`);
    Deno.exit(1);
  }
}

if (import.meta.main) {
  main();
}
