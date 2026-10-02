import { assertEquals, assertRejects, assertThrows } from "@std/testing/asserts";
import { describe, it } from "@std/testing/bdd";
import { emitOTelLog, getOTelConfig, initOTelProvider, loadRaskIngestToken } from "../../../src/infra/otel.ts";
import { trace } from "@opentelemetry/api";
import { logs } from "@opentelemetry/api-logs";

// ---------------------------------------------------------------------------
// Helper: protobuf field reader (LEN-only; matches wire-test fixture decoder)
// ---------------------------------------------------------------------------
function protoFields(b: Uint8Array): Map<number, Uint8Array[]> {
  const out = new Map<number, Uint8Array[]>();
  let p = 0;
  const num = () => {
    let n = 0, s = 0, v: number;
    do { v = b[p++]!; n += (v & 127) * 2 ** s; s += 7; } while (v & 128);
    return n;
  };
  while (p < b.length) {
    const tag = num(), f = tag >>> 3, w = tag & 7;
    if (w === 2) {
      const n = num();
      const v = b.slice(p, p + n);
      p += n;
      out.set(f, [...(out.get(f) ?? []), v]);
    } else if (w === 0) { num(); }
    else if (w === 1) { p += 8; }
    else if (w === 5) { p += 4; }
    else throw new Error(`unexpected protobuf wire type ${w}`);
  }
  return out;
}
function protoChild(b: Uint8Array, f: number): Uint8Array {
  const v = protoFields(b).get(f)?.[0];
  if (!v) throw new Error(`missing protobuf field ${f}`);
  return v;
}

// ---------------------------------------------------------------------------
// Unit tests: loadRaskIngestToken
// ---------------------------------------------------------------------------
describe("loadRaskIngestToken", () => {
  it("throws when env var is missing", () => {
    Deno.env.delete("RASK_INGEST_TOKEN_FILE");
    assertThrows(() => loadRaskIngestToken(), Error, "RASK_INGEST_TOKEN_FILE must be set");
  });

  it("throws when token file is unreadable", () => {
    Deno.env.set("RASK_INGEST_TOKEN_FILE", "/path/to/nonexistent/file/12345");
    assertThrows(() => loadRaskIngestToken(), Error, "Failed to read RASK_INGEST_TOKEN_FILE");
  });

  it("throws 'empty' when file contains only a bare newline", () => {
    const tmp = Deno.makeTempFileSync();
    try {
      Deno.writeTextFileSync(tmp, "\n");
      Deno.env.set("RASK_INGEST_TOKEN_FILE", tmp);
      assertThrows(() => loadRaskIngestToken(), Error, "RASK_INGEST_TOKEN_FILE is empty");
    } finally {
      Deno.removeSync(tmp);
    }
  });

  it("throws 'empty' when file contains only a CRLF", () => {
    const tmp = Deno.makeTempFileSync();
    try {
      Deno.writeTextFileSync(tmp, "\r\n");
      Deno.env.set("RASK_INGEST_TOKEN_FILE", tmp);
      assertThrows(() => loadRaskIngestToken(), Error, "RASK_INGEST_TOKEN_FILE is empty");
    } finally {
      Deno.removeSync(tmp);
    }
  });

  it("throws 'invalid characters' when file is whitespace/spaces (not trimmed to empty)", () => {
    // .trim() was previously used; now only one trailing newline is stripped –
    // spaces and tabs are NOT valid token68 chars and must be rejected explicitly.
    const tmp = Deno.makeTempFileSync();
    try {
      Deno.writeTextFileSync(tmp, "   \n\t  ");
      Deno.env.set("RASK_INGEST_TOKEN_FILE", tmp);
      assertThrows(() => loadRaskIngestToken(), Error, "RASK_INGEST_TOKEN_FILE contains invalid characters");
    } finally {
      Deno.removeSync(tmp);
    }
  });

  it("throws 'invalid characters' when file contains spaces", () => {
    const tmp = Deno.makeTempFileSync();
    try {
      Deno.writeTextFileSync(tmp, "invalid token with spaces");
      Deno.env.set("RASK_INGEST_TOKEN_FILE", tmp);
      assertThrows(() => loadRaskIngestToken(), Error, "RASK_INGEST_TOKEN_FILE contains invalid characters");
    } finally {
      Deno.removeSync(tmp);
    }
  });

  it("throws 'invalid characters' for pad-only token (=== has no alphabet prefix)", () => {
    const tmp = Deno.makeTempFileSync();
    try {
      Deno.writeTextFileSync(tmp, "===");
      Deno.env.set("RASK_INGEST_TOKEN_FILE", tmp);
      assertThrows(() => loadRaskIngestToken(), Error, "RASK_INGEST_TOKEN_FILE contains invalid characters");
    } finally {
      Deno.removeSync(tmp);
    }
  });

  it("throws 'invalid characters' for Unicode whitespace (U+00A0 non-breaking space)", () => {
    const tmp = Deno.makeTempFileSync();
    try {
      Deno.writeTextFileSync(tmp, "token\u00a0value");
      Deno.env.set("RASK_INGEST_TOKEN_FILE", tmp);
      assertThrows(() => loadRaskIngestToken(), Error, "RASK_INGEST_TOKEN_FILE contains invalid characters");
    } finally {
      Deno.removeSync(tmp);
    }
  });

  it("succeeds with valid RFC token (alphanumeric + padding)", () => {
    const tmp = Deno.makeTempFileSync();
    try {
      const valid = "valid_rask_token_12345==";
      Deno.writeTextFileSync(tmp, valid + "\n");
      Deno.env.set("RASK_INGEST_TOKEN_FILE", tmp);
      const loaded = loadRaskIngestToken();
      assertEquals(loaded, valid);
    } finally {
      Deno.removeSync(tmp);
    }
  });

  it("succeeds with hyphen in token (RFC 6750 token68 '-' is valid)", () => {
    // Regression: regex previously omitted '-' so URL-safe base64url tokens
    // (and generated tokens with hyphens) were wrongly rejected.
    const tmp = Deno.makeTempFileSync();
    try {
      const valid = "fixture-hyphen_token_123456";
      Deno.writeTextFileSync(tmp, valid + "\n");
      Deno.env.set("RASK_INGEST_TOKEN_FILE", tmp);
      const loaded = loadRaskIngestToken();
      assertEquals(loaded, valid);
    } finally {
      Deno.removeSync(tmp);
    }
  });

  it("strips exactly one trailing newline but preserves other content", () => {
    const tmp = Deno.makeTempFileSync();
    try {
      // Two newlines: first one is part of token? No – second \n is stripped,
      // first remains → should fail validation (not a valid token char).
      Deno.writeTextFileSync(tmp, "abc\ndef\n");
      Deno.env.set("RASK_INGEST_TOKEN_FILE", tmp);
      // "abc\ndef" contains \n which is not in the alphabet → invalid
      assertThrows(() => loadRaskIngestToken(), Error, "RASK_INGEST_TOKEN_FILE contains invalid characters");
    } finally {
      Deno.removeSync(tmp);
    }
  });
});

// ---------------------------------------------------------------------------
// Unit tests: initOTelProvider
// ---------------------------------------------------------------------------
describe("OTel Provider lifecycle", () => {
  it("does not load token when disabled", async () => {
    Deno.env.delete("RASK_INGEST_TOKEN_FILE");
    Deno.env.set("OTEL_ENABLED", "false");
    const shutdown = initOTelProvider();
    await shutdown();
  });

  it("initializes trace and log providers when enabled", async () => {
    const tmp = Deno.makeTempFileSync();
    try {
      Deno.writeTextFileSync(tmp, "valid_token_12345");
      Deno.env.set("RASK_INGEST_TOKEN_FILE", tmp);
      Deno.env.set("OTEL_ENABLED", "true");
      Deno.env.set("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318");

      const shutdown = initOTelProvider();
      await shutdown();
    } finally {
      Deno.removeSync(tmp);
      Deno.env.delete("OTEL_ENABLED");
      Deno.env.delete("RASK_INGEST_TOKEN_FILE");
    }
  });
});

// ---------------------------------------------------------------------------
// Production wire test: real initOTelProvider → local Deno.serve collector
// Verifies:
//   • Bearer header contains exact token (including hyphen)
//   • Protobuf Content-Type
//   • POST to /v1/traces and /v1/logs
//   • Inner span name and log body decoded from protobuf
//   • 307 redirect is NOT followed (credentials stay on origin)
// ---------------------------------------------------------------------------
Deno.test({
  name: "production wire: protobuf OTLP reaches local server with correct Bearer + no redirect follow",
  sanitizeOps: false,
  sanitizeResources: false,
  fn: async () => {
    const tokenFile = await Deno.makeTempFile({ dir: "/tmp" });
    Deno.env.set("RASK_INGEST_TOKEN_FILE", tokenFile);
    try {
      // Use a hyphen-containing token throughout – this is both the regression
      // check (hyphen was previously rejected) and the live Bearer token sent in
      // every OTLP POST so the wire assertions cover the real alphabet.
      const token = "fixture-hyphen_wire_token_123456";
      await Deno.writeTextFile(tokenFile, token + "\n");
      const loaded = loadRaskIngestToken();
      assertEquals(loaded, token, "hyphen token loads (regression: hyphen was rejected)");

      // Destination server: counts requests (must stay at 0 after redirect test)
      let destinationHits = 0;
      const dest = Deno.serve(
        { hostname: "127.0.0.1", port: 0, onListen: () => {} },
        () => { destinationHits++; return new Response(null, { status: 200 }); },
      );

      // Origin server: captures requests and optionally redirects to dest
      const captures: Array<{ path: string; auth: string | null; ct: string | null; body: Uint8Array }> = [];
      let redirect = false;
      const origin = Deno.serve(
        { hostname: "127.0.0.1", port: 0, onListen: () => {} },
        async (r) => {
          const path = new URL(r.url).pathname;
          const body = new Uint8Array(await r.arrayBuffer());
          captures.push({ path, auth: r.headers.get("Authorization"), ct: r.headers.get("Content-Type"), body });
          if (redirect) {
            return new Response(null, {
              status: 307,
              headers: { Location: `http://127.0.0.1:${dest.addr.port}${path}` },
            });
          }
          return new Response(null, { status: 200 });
        },
      );

      try {
        for (const mode of [false, true]) {
          redirect = mode;
          captures.length = 0;
          // Reset global providers between iterations
          trace.disable();
          logs.disable();

          const shutdown = initOTelProvider({
            enabled: true,
            serviceName: "sol_wire_fixture",
            serviceVersion: "1",
            environment: "test",
            otlpEndpoint: `http://127.0.0.1:${origin.addr.port}`,
          });

          trace.getTracer("sol-fixture").startSpan("sol-wire-span").end();
          emitOTelLog("info", "sol-wire-log");

          // Bounded shutdown: SDK flushes BatchSpanProcessor / BatchLogRecordProcessor.
          // In redirect mode the node:http transport throws OTLPExporterError on 307
          // (it never follows Location); swallow that expected error so we can still
          // assert destinationHits === 0 below.
          try {
            await Promise.race([
              shutdown(),
              new Promise<void>((_, reject) =>
                setTimeout(() => reject(new Error("shutdown timeout")), 1000)
              ),
            ]);
          } catch (e) {
            if (!redirect) throw e; // unexpected in positive mode
            // In redirect mode: exporter error is expected – SDK does not follow 307.
            const msg = e instanceof Error ? e.message : String(e);
            if (!msg.includes("307") && !msg.includes("Redirect") && !msg.includes("Temporary")) {
              throw new Error(`Unexpected shutdown error in redirect mode: ${msg}`);
            }
          }

          // Validate both signal routes
          for (const route of ["/v1/traces", "/v1/logs"]) {
            const c = captures.find((x) => x.path === route);
            if (!c) throw new Error(`missing capture for ${route} (mode redirect=${mode})`);
            assertEquals(c.auth, `Bearer ${token}`, `Bearer header for ${route}`);
            if (!c.ct?.startsWith("application/x-protobuf")) {
              throw new Error(`expected protobuf Content-Type for ${route}, got ${c.ct}`);
            }

            // Decode protobuf: ExportRequest → ResourceSpans/ResourceLogs [1] →
            // ScopeSpans/ScopeLogs [2] → Span/LogRecord [2]
            const group = protoChild(protoChild(c.body, 1), 2);
            const record = protoChild(group, 2);

            if (route.endsWith("traces")) {
              assertEquals(protoChild(record, 1).length, 16, "trace ID is 16 bytes");
              assertEquals(protoChild(record, 2).length, 8, "span ID is 8 bytes");
              assertEquals(
                new TextDecoder().decode(protoChild(record, 5)),
                "sol-wire-span",
                "span name matches",
              );
            } else {
              // LogRecord body is a AnyValue (field 5) → string_value (field 1)
              assertEquals(
                new TextDecoder().decode(protoChild(protoChild(record, 5), 1)),
                "sol-wire-log",
                "log body matches",
              );
            }
          }

          assertEquals(destinationHits, 0, "redirect destination must not receive any traffic");
          console.log(
            mode
              ? "307-redirect: origin received credentials; destination received 0 requests ✓"
              : "positive: Bearer + protobuf inner span/log verified ✓",
          );
        }
      } finally {
        await origin.shutdown();
        await dest.shutdown();
        trace.disable();
        logs.disable();
      }
    } finally {
      await Deno.remove(tokenFile);
      Deno.env.delete("RASK_INGEST_TOKEN_FILE");
    }
  },
});
