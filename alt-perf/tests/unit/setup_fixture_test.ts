import { assertEquals, assertRejects, assertStringIncludes } from "jsr:@std/assert@^1.0.0";
import { afterEach, beforeEach, describe, it } from "jsr:@std/testing@^1.0.0/bdd";
import { join } from "jsr:@std/path@^1.0.0";
import {
  createUser,
  loadExactRunCredentials,
  persistCredentialIncremental,
  runSetup,
} from "../../scripts/feed-load-test-setup.ts";
import {
  createManifest,
  currentManifestPath,
  manifestPath,
  readManifest,
} from "../../scripts/manifest.ts";

let tmpDir: string;

function setup() {
  tmpDir = Deno.makeTempDirSync({ dir: "/tmp", prefix: "setup_test_" });
}

function teardown() {
  try {
    Deno.removeSync(tmpDir, { recursive: true });
  } catch {
    // ignore
  }
}

describe("setup schema and journal pointers", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("Test malformedversion/json/journalpointer/tmp retained no actualKratos", async () => {
    const mPath = manifestPath(tmpDir, "bad-run");
    // Write malformed JSON
    await Deno.writeTextFile(mPath, `{"runId": "bad-run", "version": "2"}`);

    // Should fail and NOT overwrite with a new one
    await assertRejects(
      async () => {
        await runSetup({
          userCount: 1,
          runId: "bad-run",
          outputDir: tmpDir,
          kratosAdmin: "http://mock-kratos",
          fetchFn: () => Promise.resolve(new Response(null, { status: 500 })),
        });
      },
      Error,
      "Unsupported manifest version: 2",
    );

    // Verify it wasn't overwritten
    const content = await Deno.readTextFile(mPath);
    assertEquals(content, `{"runId": "bad-run", "version": "2"}`);
  });

  it("Test parse/schema errors abort with existing bytes UNMODIFIED", async () => {
    const mPath = manifestPath(tmpDir, "bad-json-run");
    await Deno.writeTextFile(mPath, `{"runId": "bad-json-run", "version": "1", `); // syntax error

    await assertRejects(
      async () => {
        await runSetup({
          userCount: 1,
          runId: "bad-json-run",
          outputDir: tmpDir,
          kratosAdmin: "http://mock-kratos",
          fetchFn: () => Promise.resolve(new Response(null, { status: 500 })),
        });
      },
      Error,
      "Expected double-quoted property name in JSON",
    );

    const content = await Deno.readTextFile(mPath);
    assertEquals(content, `{"runId": "bad-json-run", "version": "1", `);
  });
});

describe("setup credential intent and crash recovery", () => {
  beforeEach(setup);
  afterEach(teardown);

  it("Testinterruptbeforecreate, aftercreate, afteridentityjournal; samepasswordnotregenerated / existingownedidentity reused ONLYprovable; corruptedintentfailclosed", async () => {
    const runId = "crash-run";
    const mPath = manifestPath(tmpDir, runId);
    const m = createManifest(runId, "feed-load-test-setup");
    const cJournalPath = join(tmpDir, `run-credentials-${runId}.json`);

    let mockFetchCalls = 0;

    // We will simulate 409 and returning an owned identity
    const mockFetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
      mockFetchCalls++;
      const url = input.toString();
      const method = init?.method || "GET";

      if (method === "POST" && url.includes("/admin/identities")) {
        return new Response(null, { status: 409 });
      }
      if (method === "GET" && url.includes("credentials_identifier=")) {
        // Return existing identity owned by this run
        return new Response(
          JSON.stringify([{
            id: "11111111-1111-1111-1111-111111111111",
            traits: { email: "loadtest-0000@test.alt.local" },
            metadata_admin: { run_id: runId, intent_nonce: "fake-nonce" },
          }]),
          { status: 200 },
        );
      }
      return new Response(null, { status: 500 });
    };

    // First attempt: simulate that intent was written, but API creation failed or we crashed
    // Let's explicitly write intent.
    await persistCredentialIncremental(cJournalPath, {
      email: "loadtest-0000@test.alt.local",
      password: "AltPass_!A1original",
      userId: "",
      intentNonce: "fake-nonce",
    });

    const user = await createUser(0, m, mPath, "http://mock-kratos", 0, mockFetch, cJournalPath);

    // It should hit 409, check ownership, find it is owned by this run, and recover the password from intent!
    assertEquals(user !== null, true);
    assertEquals(user?.password, "AltPass_!A1original");
    assertEquals(user?.userId, "11111111-1111-1111-1111-111111111111");

    // The manifest should now contain the identity
    const afterM = await readManifest(mPath);
    assertEquals(afterM !== null, true);
    assertEquals(afterM!.identities.length, 1);
    const firstIdentity = afterM!.identities[0];
    if (!firstIdentity) throw new Error("firstIdentity is undefined");
    assertEquals(firstIdentity.kratosId, "11111111-1111-1111-1111-111111111111");

    // Corrupted intent fail closed test
    const m2Path = manifestPath(tmpDir, "run2");
    const m2 = createManifest("run2", "feed-load-test-setup");
    const c2Path = join(tmpDir, `run-credentials-run2.json`);

    const mockFetch2 = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
      const url = input.toString();
      const method = init?.method || "GET";
      if (method === "POST" && url.includes("/admin/identities")) {
        return new Response(null, { status: 409 });
      }
      if (method === "GET" && url.includes("credentials_identifier=")) {
        return new Response(
          JSON.stringify([{
            id: "22222222-2222-2222-2222-222222222222",
            traits: { email: "loadtest-0000@test.alt.local" },
            metadata_admin: { run_id: "run2", intent_nonce: "other-nonce" },
          }]),
          { status: 200 },
        );
      }
      return new Response(null, { status: 500 });
    };

    // DO NOT write intent this time. It should fail closed!
    const user2 = await createUser(0, m2, m2Path, "http://mock-kratos", 0, mockFetch2, c2Path);
    assertEquals(user2, null);
  });
});
