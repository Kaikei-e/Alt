#!/usr/bin/env -S deno run --allow-net --allow-write --allow-read --allow-env
// feed-load-test-setup.ts - Create test users in Kratos for feed registration load test
//
// Creates N users via Kratos Admin API with email/password credentials.
// Outputs user list as JSON + CSV for K6 SharedArray consumption.
//
// Usage:
//   deno run --allow-net --allow-write --allow-read --allow-env alt-perf/scripts/feed-load-test-setup.ts
//   deno run --allow-net --allow-write --allow-read --allow-env alt-perf/scripts/feed-load-test-setup.ts --count=10

import { parseArgs } from "jsr:@std/cli@^1.0.0/parse-args";

const args = parseArgs(Deno.args, {
  default: { count: 1000 },
  alias: { n: "count" },
});

const USER_COUNT = Number(args.count);
const KRATOS_ADMIN = Deno.env.get("KRATOS_ADMIN_URL") || "http://localhost:4434";
const BATCH_SIZE = 50;
const OUTPUT_DIR = new URL("../k6/data/", import.meta.url).pathname;

interface TestUser {
  email: string;
  password: string;
  userId: string;
}

async function createUser(
  index: number,
  retries = 1,
): Promise<TestUser | null> {
  const email = `loadtest-${String(index).padStart(4, "0")}@test.alt.local`;
  const password = crypto.randomUUID() + crypto.randomUUID();

  const body = {
    schema_id: "default",
    traits: { email },
    credentials: {
      password: {
        config: { password },
      },
    },
    state: "active",
  };

  for (let attempt = 0; attempt <= retries; attempt++) {
    try {
      const res = await fetch(`${KRATOS_ADMIN}/admin/identities`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });

      if (res.status === 201 || res.status === 200) {
        const data = await res.json();
        return { email, password, userId: data.id };
      }

      // 409 Conflict = user already exists → delete and recreate with known password
      if (res.status === 409) {
        console.warn(`User ${email} already exists, recreating...`);
        const listRes = await fetch(
          `${KRATOS_ADMIN}/admin/identities?credentials_identifier=${email}`,
        );
        if (listRes.ok) {
          const identities = await listRes.json();
          if (identities.length > 0) {
            // Delete existing identity
            await fetch(
              `${KRATOS_ADMIN}/admin/identities/${identities[0].id}`,
              { method: "DELETE" },
            );
            // Retry creation (will use the password generated above)
            const retryRes = await fetch(`${KRATOS_ADMIN}/admin/identities`, {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify(body),
            });
            if (retryRes.status === 201 || retryRes.status === 200) {
              const data = await retryRes.json();
              return { email, password, userId: data.id };
            }
            const errBody = await retryRes.text();
            console.error(
              `Failed to recreate ${email}: status=${retryRes.status} body=${errBody}`,
            );
          }
        }
        return null;
      }

      const errBody = await res.text();
      console.error(
        `Failed to create ${email}: status=${res.status} body=${errBody}`,
      );

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

async function main() {
  console.log(
    `Creating ${USER_COUNT} test users via ${KRATOS_ADMIN}...`,
  );

  const users: TestUser[] = [];
  let failed = 0;

  for (let batchStart = 0; batchStart < USER_COUNT; batchStart += BATCH_SIZE) {
    const batchEnd = Math.min(batchStart + BATCH_SIZE, USER_COUNT);
    const promises: Promise<TestUser | null>[] = [];

    for (let i = batchStart; i < batchEnd; i++) {
      promises.push(createUser(i));
    }

    const results = await Promise.all(promises);
    for (const result of results) {
      if (result) {
        users.push(result);
      } else {
        failed++;
      }
    }

    if ((batchStart + BATCH_SIZE) % 100 === 0 || batchEnd === USER_COUNT) {
      console.log(
        `Progress: ${users.length}/${USER_COUNT} created, ${failed} failed`,
      );
    }
  }

  // Ensure output directory exists
  await Deno.mkdir(OUTPUT_DIR, { recursive: true });

  // Write JSON (for K6 SharedArray)
  const jsonPath = `${OUTPUT_DIR}/load-test-users.json`;
  await Deno.writeTextFile(jsonPath, JSON.stringify(users, null, 2));
  console.log(`Written ${jsonPath}`);

  // Write CSV (for reference / teardown)
  const csvPath = `${OUTPUT_DIR}/load-test-users.csv`;
  const csvLines = ["email,password,userId"];
  for (const u of users) {
    csvLines.push(`${u.email},${u.password},${u.userId}`);
  }
  await Deno.writeTextFile(csvPath, csvLines.join("\n") + "\n");
  console.log(`Written ${csvPath}`);

  console.log(
    `\nSetup complete: ${users.length} users created, ${failed} failed`,
  );

  if (failed > 0) {
    Deno.exit(1);
  }
}

main();
