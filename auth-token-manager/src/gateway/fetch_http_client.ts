/**
 * HTTP client gateway with timeout and proxy support
 */

import type { HttpClient } from "../port/http_client.ts";
import type { NetworkConfig } from "../domain/types.ts";

export class FetchHttpClient implements HttpClient {
  constructor(private networkConfig: NetworkConfig) {}

  async fetch(url: string, options: RequestInit = {}): Promise<Response> {
    const controller = new AbortController();
    const timeoutId = setTimeout(
      () => controller.abort(),
      this.networkConfig.http_timeout,
    );

    // Deno's fetch honors HTTP_PROXY/HTTPS_PROXY automatically, so a
    // configured proxy needs no extra handling here. Sending the request
    // exactly once matters: OAuth token-exchange/refresh calls are
    // non-idempotent (refresh_token rotates), so a duplicate send would
    // resend an already-rotated token and lose it.
    const fallbackToDirect = Deno.env.get("NETWORK_FALLBACK_TO_DIRECT") ===
      "true";
    const hasProxyConfigured = Boolean(
      Deno.env.get("HTTP_PROXY") || Deno.env.get("HTTPS_PROXY"),
    );

    // Deno reads HTTP_PROXY/HTTPS_PROXY once at process start for the
    // default fetch client, so deleting the env vars at request time has no
    // effect and mutating global env would race concurrent requests.
    // A per-request client with no proxy configured bypasses the proxy
    // without touching global state.
    const directClient = fallbackToDirect && hasProxyConfigured
      ? Deno.createHttpClient({})
      : undefined;

    const fetchOptions: RequestInit = {
      ...options,
      signal: controller.signal,
      ...(directClient ? { client: directClient } : {}),
    };

    try {
      return await globalThis.fetch(url, fetchOptions);
    } catch (error) {
      if (error instanceof Error && error.name === "AbortError") {
        throw new Error(
          `HTTP request timed out after ${this.networkConfig.http_timeout}ms: ${url}`,
        );
      }
      throw error;
    } finally {
      clearTimeout(timeoutId);
      directClient?.close();
    }
  }
}
