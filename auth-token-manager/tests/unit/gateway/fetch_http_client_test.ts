import { afterEach, describe, it } from "@std/testing/bdd";
import { assertEquals, assertRejects } from "@std/testing/asserts";
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

  it("should forward the exact url, method, headers, body and an abort signal to fetch", async () => {
    const fetchStub = stub(
      globalThis,
      "fetch",
      () => Promise.resolve(new Response("ok", { status: 200 })),
    );

    try {
      const client = new FetchHttpClient(networkConfig);
      await client.fetch("https://example.com/api/token", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: "refresh_token=abc",
      });

      assertEquals(fetchStub.calls.length, 1);
      const [calledUrl, calledOptions] = fetchStub.calls[0].args as [
        string,
        RequestInit,
      ];
      assertEquals(calledUrl, "https://example.com/api/token");
      assertEquals(calledOptions.method, "POST");
      assertEquals(calledOptions.headers, {
        "Content-Type": "application/x-www-form-urlencoded",
      });
      assertEquals(calledOptions.body, "refresh_token=abc");
      // A garbled/missing signal would silently disable the http_timeout.
      assertEquals(calledOptions.signal instanceof AbortSignal, true);
    } finally {
      fetchStub.restore();
    }
  });

  it("should throw a descriptive timeout error (not the raw AbortError) when the request aborts", async () => {
    const abortError = new Error("The signal has been aborted");
    abortError.name = "AbortError";

    const fetchStub = stub(
      globalThis,
      "fetch",
      () => Promise.reject(abortError),
    );

    try {
      const client = new FetchHttpClient(networkConfig);
      await assertRejects(
        () => client.fetch("https://example.com/api/token"),
        Error,
        `HTTP request timed out after ${networkConfig.http_timeout}ms: https://example.com/api/token`,
      );
    } finally {
      fetchStub.restore();
    }
  });

  it("should propagate non-timeout fetch errors unchanged", async () => {
    const fetchStub = stub(
      globalThis,
      "fetch",
      () => Promise.reject(new Error("network unreachable")),
    );

    try {
      const client = new FetchHttpClient(networkConfig);
      await assertRejects(
        () => client.fetch("https://example.com/api/token"),
        Error,
        "network unreachable",
      );
    } finally {
      fetchStub.restore();
    }
  });

  it("should route through a direct (non-proxied) Deno.createHttpClient and close it afterwards when NETWORK_FALLBACK_TO_DIRECT=true and a proxy is configured", async () => {
    Deno.env.set("HTTPS_PROXY", "http://proxy.internal:8080");
    Deno.env.set("NETWORK_FALLBACK_TO_DIRECT", "true");

    let closed = false;
    const fakeDirectClient = {
      close: () => {
        closed = true;
      },
    } as unknown as Deno.HttpClient;

    const createHttpClientStub = stub(
      Deno,
      "createHttpClient",
      () => fakeDirectClient,
    );
    const fetchStub = stub(
      globalThis,
      "fetch",
      () => Promise.resolve(new Response("ok", { status: 200 })),
    );

    try {
      const client = new FetchHttpClient(networkConfig);
      await client.fetch("https://www.inoreader.com/oauth2/token", {
        method: "POST",
      });

      assertEquals(createHttpClientStub.calls.length, 1);
      const [, calledOptions] = fetchStub.calls[0].args as [
        string,
        RequestInit & { client?: unknown },
      ];
      assertEquals(calledOptions.client, fakeDirectClient);
      // The per-request client must not leak past the request.
      assertEquals(closed, true);
    } finally {
      fetchStub.restore();
      createHttpClientStub.restore();
    }
  });
});
