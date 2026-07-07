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

    try {
      const fallbackToDirect = Deno.env.get("NETWORK_FALLBACK_TO_DIRECT") ===
        "true";

      const fetchOptions: RequestInit = {
        ...options,
        signal: controller.signal,
      };

      // Deno's fetch honors HTTP_PROXY/HTTPS_PROXY automatically, so a
      // configured proxy needs no extra handling here. Sending the request
      // exactly once matters: OAuth token-exchange/refresh calls are
      // non-idempotent (refresh_token rotates), so a duplicate send would
      // resend an already-rotated token and lose it.
      const originalHttpProxy = Deno.env.get("HTTP_PROXY");
      const originalHttpsProxy = Deno.env.get("HTTPS_PROXY");

      if (
        fallbackToDirect && (originalHttpProxy || originalHttpsProxy)
      ) {
        if (originalHttpProxy) Deno.env.delete("HTTP_PROXY");
        if (originalHttpsProxy) Deno.env.delete("HTTPS_PROXY");
      }

      try {
        const response = await globalThis.fetch(url, fetchOptions);
        return response;
      } finally {
        if (fallbackToDirect) {
          if (originalHttpProxy) Deno.env.set("HTTP_PROXY", originalHttpProxy);
          if (originalHttpsProxy) {
            Deno.env.set("HTTPS_PROXY", originalHttpsProxy);
          }
        }
      }
    } catch (error) {
      if (error instanceof Error && error.name === "AbortError") {
        throw new Error(
          `HTTP request timed out after ${this.networkConfig.http_timeout}ms: ${url}`,
        );
      }
      throw error;
    } finally {
      clearTimeout(timeoutId);
    }
  }
}
