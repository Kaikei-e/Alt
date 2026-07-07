import { afterEach, describe, it } from "@std/testing/bdd";
import { assertEquals } from "@std/testing/asserts";
import { stub } from "@std/testing/mock";
import { FetchHttpClient } from "../../../src/gateway/fetch_http_client.ts";
import type { NetworkConfig } from "../../../src/domain/types.ts";

const networkConfig: NetworkConfig = {
  http_timeout: 5000,
  connectivity_check: false,
  connectivity_timeout: 5000,
};

describe("FetchHttpClient", {
  sanitizeResources: false,
  sanitizeOps: false,
}, () => {
  afterEach(() => {
    Deno.env.delete("HTTP_PROXY");
    Deno.env.delete("HTTPS_PROXY");
    Deno.env.delete("NETWORK_FALLBACK_TO_DIRECT");
  });

  it("should send the request exactly once when no proxy is configured", async () => {
    const fetchStub = stub(
      globalThis,
      "fetch",
      () => Promise.resolve(new Response("ok", { status: 200 })),
    );

    try {
      const client = new FetchHttpClient(networkConfig);
      await client.fetch("https://example.com/api/token", {
        method: "POST",
        body: "refresh_token=abc",
      });

      assertEquals(fetchStub.calls.length, 1);
    } finally {
      fetchStub.restore();
    }
  });

  it("should send a proxied POST exactly once, not twice, when a proxy is configured", async () => {
    Deno.env.set("HTTPS_PROXY", "http://proxy.internal:8080");

    const fetchStub = stub(
      globalThis,
      "fetch",
      () => Promise.resolve(new Response("ok", { status: 200 })),
    );

    try {
      const client = new FetchHttpClient(networkConfig);
      await client.fetch("https://www.inoreader.com/oauth2/token", {
        method: "POST",
        body: "grant_type=refresh_token&refresh_token=rotates-once",
      });

      // Non-idempotent OAuth token-exchange/refresh calls must never be
      // sent twice — a duplicate send would resend an already-rotated
      // refresh_token and lose it (invalid_grant).
      assertEquals(fetchStub.calls.length, 1);
    } finally {
      fetchStub.restore();
    }
  });
});
