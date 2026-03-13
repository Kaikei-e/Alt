#!/usr/bin/env -S deno run --allow-net --allow-read --allow-write --allow-env --allow-run
// feed-load-test-teardown.ts - Clean up load test data
//
// 1. Delete Kratos identities
// 2. Clean up DB records (feeds, feed_links created by mock-rss-server)
// 3. Remove credential files
//
// Usage:
//   deno run --allow-net --allow-read --allow-write --allow-env --allow-run \
//     alt-perf/scripts/feed-load-test-teardown.ts

const KRATOS_ADMIN = Deno.env.get("KRATOS_ADMIN_URL") || "http://localhost:4434";
const BATCH_SIZE = parseInt(Deno.env.get("TEARDOWN_BATCH_SIZE") || "50", 10);
const DATA_DIR = new URL("../k6/data/", import.meta.url).pathname;
const COMPOSE_CMD = Deno.env.get("COMPOSE_CMD") ||
  "docker compose -f compose/compose.yaml -p alt";

interface TestUser {
  email: string;
  password: string;
  userId: string;
}

async function loadUsers(): Promise<TestUser[]> {
  const jsonPath = `${DATA_DIR}/load-test-users.json`;
  try {
    const text = await Deno.readTextFile(jsonPath);
    return JSON.parse(text);
  } catch {
    console.error(`Cannot read ${jsonPath}. Was setup run?`);
    return [];
  }
}

async function deleteIdentity(userId: string, retries = 1): Promise<boolean> {
  for (let attempt = 0; attempt <= retries; attempt++) {
    try {
      const res = await fetch(
        `${KRATOS_ADMIN}/admin/identities/${userId}`,
        { method: "DELETE" },
      );
      if (res.status === 204 || res.status === 200 || res.status === 404) {
        return true;
      }
      console.error(
        `Delete identity ${userId}: status=${res.status}`,
      );
    } catch (e) {
      console.error(`Delete identity ${userId}: ${e}`);
    }
    if (attempt < retries) {
      await new Promise((r) => setTimeout(r, 500));
    }
  }
  return false;
}

async function deleteKratosUsers(users: TestUser[]): Promise<void> {
  console.log(`Deleting ${users.length} Kratos identities...`);
  let deleted = 0;
  let failed = 0;

  for (let i = 0; i < users.length; i += BATCH_SIZE) {
    const batch = users.slice(i, i + BATCH_SIZE);
    const results = await Promise.all(
      batch.map((u) => deleteIdentity(u.userId)),
    );
    for (const ok of results) {
      if (ok) deleted++;
      else failed++;
    }
    if ((i + BATCH_SIZE) % 200 === 0 || i + BATCH_SIZE >= users.length) {
      console.log(`  Progress: ${deleted} deleted, ${failed} failed`);
    }
  }
  console.log(
    `Kratos cleanup: ${deleted} deleted, ${failed} failed`,
  );
}

async function cleanupDatabase(): Promise<void> {
  console.log("Cleaning up database records...");

  // Extend statement_timeout for large-scale teardown (1M+ rows)
  const setTimeoutSQL = `SET statement_timeout = '300s';`;

  // Step 3a: Delete feeds linked to mock-rss-server feed_links
  // feeds.feed_link_id has ON DELETE SET NULL, so we must delete feeds first
  const deleteFeedsSQL = `
    ${setTimeoutSQL}
    DELETE FROM feeds
    WHERE feed_link_id IN (
      SELECT id FROM feed_links WHERE url LIKE 'http://mock-rss-%:8080/%'
    );
  `;

  // Step 3b: Delete feed_links (cascades to user_feed_subscriptions, feed_link_availability)
  const deleteFeedLinksSQL = `
    ${setTimeoutSQL}
    DELETE FROM feed_links WHERE url LIKE 'http://mock-rss-%:8080/%';
  `;

  const composeParts = COMPOSE_CMD.split(" ");

  for (const [label, sql] of [
    ["feeds", deleteFeedsSQL],
    ["feed_links", deleteFeedLinksSQL],
  ] as const) {
    console.log(`  Deleting ${label}...`);
    const dbService = Deno.env.get("DB_SERVICE") || "db";
    const dbUser = Deno.env.get("POSTGRES_USER") || "alt_db_user";
    const dbName = Deno.env.get("POSTGRES_DB") || "alt";
    const cmd = new Deno.Command(composeParts[0], {
      args: [
        ...composeParts.slice(1),
        "exec",
        "-T",
        dbService,
        "psql",
        "-U",
        dbUser,
        "-d",
        dbName,
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
    } else {
      console.error(`  ${label} failed: ${stderr}`);
    }
  }
}

async function removeDataFiles(): Promise<void> {
  console.log("Removing data files...");
  for (const file of ["load-test-users.json", "load-test-users.csv"]) {
    const path = `${DATA_DIR}/${file}`;
    try {
      await Deno.remove(path);
      console.log(`  Removed ${path}`);
    } catch {
      console.log(`  ${path} not found, skipping`);
    }
  }
}

async function main() {
  console.log("=== Feed Load Test Teardown ===\n");

  // Step 1: Load users
  const users = await loadUsers();

  // Step 2: Delete Kratos identities
  if (users.length > 0) {
    await deleteKratosUsers(users);
  } else {
    console.log("No users to delete from Kratos.");
  }

  // Step 3: Database cleanup
  await cleanupDatabase();

  // Step 4: Remove data files
  await removeDataFiles();

  console.log("\nTeardown complete.");
}

main();
