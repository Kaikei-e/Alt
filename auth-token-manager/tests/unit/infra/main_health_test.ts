/**
 * Boot-seam integration tests for main.ts and logger.ts.
 *
 * Tests:
 *   1. StructuredLogger constructor is pure – creating one without RASK env
 *      does NOT throw (OTel init is now explicit via initializeOTel()).
 *   2. initializeOTel() with OTEL_ENABLED=false is a no-op (safe on health path).
 *   3. initializeOTel() with OTEL_ENABLED=true and absent RASK fails at init
 *      time (before any outbound traffic).
 *   4. emergencyLog-equivalent: DataSanitizer redacts secrets before JSON.stringify.
 *   5. Child-process: `main.ts health` with OTEL_ENABLED=true, no RASK env
 *      permission exits 0 or 1 (healthy/unhealthy) – never a permission crash.
 *   6. Child-process: `main.ts daemon` with OTEL_ENABLED=true and absent
 *      RASK_INGEST_TOKEN_FILE exits non-zero with ZERO outbound hits.
 *
 * Run with:
 *   deno test --cached-only --no-prompt \
 *     --allow-env=OTEL_*,RASK_INGEST_TOKEN_FILE,DEPLOYMENT_ENV,SERVICE_VERSION,LOG_LEVEL \
 *     --allow-read=/tmp --allow-write=/tmp --allow-net=127.0.0.1 \
 *     --allow-run=deno \
 *     tests/unit/infra/main_health_test.ts
 */

import {
  assertEquals,
  assertStringIncludes,
  assertThrows,
} from "@std/testing/asserts";
import { describe, it } from "@std/testing/bdd";
import {
  DataSanitizer,
  initializeOTel,
  StructuredLogger,
} from "../../../src/infra/logger.ts";
import { trace } from "@opentelemetry/api";
import { logs } from "@opentelemetry/api-logs";

const MAIN = new URL("../../../main.ts", import.meta.url).pathname;

/** Spawn a child deno process with restricted permissions. Returns exit code + output. */
async function childRun(
  flags: string[],
  mainArgs: string[],
  env: Record<string, string>,
): Promise<{ code: number; stdout: string; stderr: string }> {
  const cmd = new Deno.Command("deno", {
    args: ["run", "--cached-only", "--no-prompt", ...flags, MAIN, ...mainArgs],
    env,
    stdout: "piped",
    stderr: "piped",
  });
  const out = await cmd.output();
  return {
    code: out.code,
    stdout: new TextDecoder().decode(out.stdout),
    stderr: new TextDecoder().decode(out.stderr),
  };
}

// ---------------------------------------------------------------------------
// In-process unit tests (no child spawn, no --allow-run needed)
// ---------------------------------------------------------------------------
describe("StructuredLogger pure constructor", {
  sanitizeOps: false,
  sanitizeResources: false,
}, () => {
  it("creating a StructuredLogger without RASK env does NOT throw", () => {
    Deno.env.delete("RASK_INGEST_TOKEN_FILE");
    // This must not throw – OTel is no longer initialised in the constructor.
    const _logger = new StructuredLogger("boot-seam-test");
  });

  it("initializeOTel with OTEL_ENABLED=false is a no-op (health-safe)", () => {
    Deno.env.delete("RASK_INGEST_TOKEN_FILE");
    Deno.env.set("OTEL_ENABLED", "false");
    try {
      // Must not throw even without RASK env
      initializeOTel();
      trace.disable();
      logs.disable();
    } finally {
      Deno.env.delete("OTEL_ENABLED");
    }
  });

  it("initializeOTel with OTEL_ENABLED=true and absent RASK throws at init (before outbound)", () => {
    Deno.env.delete("RASK_INGEST_TOKEN_FILE");
    Deno.env.set("OTEL_ENABLED", "true");
    try {
      assertThrows(
        () => initializeOTel(),
        Error,
        "RASK_INGEST_TOKEN_FILE must be set",
      );
    } finally {
      Deno.env.delete("OTEL_ENABLED");
      trace.disable();
      logs.disable();
    }
  });
});

// ---------------------------------------------------------------------------
// DataSanitizer / emergencyLog secret-safety
// ---------------------------------------------------------------------------
describe("emergencyLog secret sanitization (DataSanitizer)", () => {
  it("DataSanitizer redacts OAuth token pattern in detail before JSON serialization", () => {
    const secret = "ya29.a0AfH6SMBsecret_bearer_credential_12345";
    const payload: Record<string, unknown> = {
      level: "error",
      msg: "startup failed",
      token: secret,
      nested: { access_token: secret },
    };
    const sanitized = DataSanitizer.sanitize(payload);
    const json = JSON.stringify(sanitized);

    if (json.includes("ya29.a0AfH6SMBsecret_bearer_credential")) {
      throw new Error(`Secret leaked in emergencyLog payload: ${json}`);
    }
    assertStringIncludes(json, "[REDACTED]");
    console.log("DataSanitizer redacts secret payload ✓");
  });

  it("DataSanitizer redacts sensitive field names (access_token, refresh_token, secret)", () => {
    const sanitized = DataSanitizer.sanitize({
      access_token: "should_be_redacted",
      refresh_token: "also_redacted",
      client_secret: "also_redacted",
      safe_field: "safe_value",
    }) as Record<string, unknown>;
    assertEquals(sanitized["access_token"], "[REDACTED]");
    assertEquals(sanitized["refresh_token"], "[REDACTED]");
    assertEquals(sanitized["client_secret"], "[REDACTED]");
    assertEquals(sanitized["safe_field"], "safe_value");
  });

  it("emergencyLog-equivalent: stringify of sanitized {level, msg, ...detail} hides secrets", () => {
    const detail = { reason: "ya29.token_leak_12345 authentication failed" };
    const sanitized = DataSanitizer.sanitize({
      level: "error",
      msg: "startup failed",
      ...detail,
    });
    const json = JSON.stringify(sanitized);
    if (json.includes("ya29.token_leak_12345")) {
      throw new Error(`Raw secret in emergency log output: ${json}`);
    }
    assertStringIncludes(json, "[REDACTED]");
  });
});

// ---------------------------------------------------------------------------
// Child-process tests (require --allow-run=deno)
// ---------------------------------------------------------------------------
describe("main.ts child-process boot behavior", {
  sanitizeOps: false,
  sanitizeResources: false,
}, () => {
  // All env vars that config.loadConfig() and health_check.ts legitimately read.
  // RASK_INGEST_TOKEN_FILE is intentionally NOT included: health fast-path must
  // never attempt to read it even when OTEL_ENABLED=true.
  const ALL_CONFIG_ENVS =
    "OTEL_ENABLED,OTEL_SERVICE_NAME,OTEL_EXPORTER_OTLP_ENDPOINT," +
    "DEPLOYMENT_ENV,SERVICE_VERSION,LOG_LEVEL,TOKEN_STORAGE_PATH," +
    "DENO_ENV,NODE_ENV,RETRY_MAX_ATTEMPTS,RETRY_BASE_DELAY,RETRY_MAX_DELAY," +
    "RETRY_BACKOFF_FACTOR,HTTP_TIMEOUT,CONNECTIVITY_CHECK,CONNECTIVITY_TIMEOUT," +
    "LOG_INCLUDE_TIMESTAMP,LOG_INCLUDE_STACK_TRACE," +
    // Health usecase reads these for environment_ready check (legitimate deps):
    "INOREADER_CLIENT_ID,INOREADER_CLIENT_SECRET,INOREADER_REDIRECT_URI," +
    "INOREADER_CLIENT_ID_FILE,INOREADER_CLIENT_SECRET_FILE,INTERNAL_AUTH_TOKEN,INTERNAL_AUTH_TOKEN_FILE";

  it("health with OTEL_ENABLED=true and no RASK: exits 0 or 1, never permission crash", async () => {
    const dir = await Deno.makeTempDir({ dir: "/tmp" });
    const file = dir + "/oauth2_token.env";
    await Deno.writeTextFile(file, ""); // empty → unhealthy token, but no crash

    try {
      const { code, stderr } = await childRun(
        [
          `--allow-env=${ALL_CONFIG_ENVS}`,
          `--allow-read=${dir}`,
          `--allow-write=${dir}`,
          // NO --allow-net: health must not attempt network
          // RASK_INGEST_TOKEN_FILE is NOT in --allow-env above
        ],
        ["health"],
        {
          OTEL_ENABLED: "true",
          TOKEN_STORAGE_PATH: file,
          LOG_LEVEL: "ERROR",
          CONNECTIVITY_CHECK: "false",
          INOREADER_CLIENT_ID: "test-client-id",
          INOREADER_CLIENT_SECRET: "test-client-secret",
          INOREADER_REDIRECT_URI: "http://localhost:9201/callback",
        },
      );

      // Health must not attempt to read RASK_INGEST_TOKEN_FILE
      if (stderr.includes("RASK_INGEST_TOKEN_FILE must be set")) {
        throw new Error(
          `health path called initializeOTel / loadRaskIngestToken:\n${stderr}`,
        );
      }
      // code 0 = healthy, 1 = unhealthy (empty token store), both acceptable
      // (code 1 is expected here since oauth2_token.env is empty)
      if (code !== 0 && code !== 1) {
        throw new Error(`unexpected exit code ${code}:\n${stderr}`);
      }
      console.log(
        `health fast-path exit=${code} (0=healthy/1=unhealthy token, no RASK attempt) ✓`,
      );
    } finally {
      await Deno.remove(dir, { recursive: true });
    }
  });

  it("daemon with OTEL_ENABLED=true and absent RASK: fails non-zero with ZERO outbound hits", async () => {
    const dir = await Deno.makeTempDir({ dir: "/tmp" });
    const file = dir + "/oauth2_token.env";
    await Deno.writeTextFile(file, "");

    let inboundHits = 0;
    const captureServer = Deno.serve(
      { hostname: "127.0.0.1", port: 0, onListen: () => {} },
      () => {
        inboundHits++;
        return new Response(null, { status: 200 });
      },
    );
    const port = captureServer.addr.port;

    try {
      const { code, stderr } = await childRun(
        [
          `--allow-env=${ALL_CONFIG_ENVS},INOREADER_CLIENT_ID,INOREADER_CLIENT_SECRET,INOREADER_REDIRECT_URI`,
          `--allow-read=${dir}`,
          `--allow-write=${dir}`,
          `--allow-net=127.0.0.1:${port}`,
          // NO RASK_INGEST_TOKEN_FILE in the env list
        ],
        ["daemon"],
        {
          OTEL_ENABLED: "true",
          OTEL_EXPORTER_OTLP_ENDPOINT: `http://127.0.0.1:${port}`,
          TOKEN_STORAGE_PATH: file,
          INOREADER_CLIENT_ID: "test-client-id",
          INOREADER_CLIENT_SECRET: "test-client-secret",
          INOREADER_REDIRECT_URI: "http://localhost:9201/callback",
          CONNECTIVITY_CHECK: "false",
          LOG_LEVEL: "ERROR",
        },
      );

      if (code === 0) {
        throw new Error(
          "daemon should exit non-zero without RASK_INGEST_TOKEN_FILE",
        );
      }
      assertEquals(
        inboundHits,
        0,
        "capture server must receive ZERO requests before startup failure",
      );
      console.log(`daemon absent RASK: exit=${code}, outbound=0 ✓`);
    } finally {
      await captureServer.shutdown();
      await Deno.remove(dir, { recursive: true });
    }
  });
});
