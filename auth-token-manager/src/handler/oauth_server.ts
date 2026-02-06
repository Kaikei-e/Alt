/**
 * OAuth callback HTTP server handler
 * Security: /api/token requires auth header, state validation with TTL
 */

import type { AuthorizeUsecase } from "../usecase/authorize.ts";
import type { SecretManager } from "../port/secret_manager.ts";
import type { InoreaderCredentials } from "../domain/types.ts";
import { logger } from "../infra/logger.ts";

interface PendingAuthState {
  state: string;
  createdAt: number;
}

const STATE_TTL_MS = 5 * 60 * 1000; // 5 minutes

export class OAuthServer {
  private pendingStates = new Map<string, PendingAuthState>();

  constructor(
    private authorizeUsecase: AuthorizeUsecase,
    private secretManager: SecretManager,
    private credentials: InoreaderCredentials,
  ) {}

  start(signal?: AbortSignal): void {
    const redirectUrl = new URL(this.credentials.redirect_uri);

    logger.info(
      `Starting persistent OAuth callback listener at 0.0.0.0:${
        redirectUrl.port || 80
      }`,
    );

    Deno.serve(
      {
        port: Number(redirectUrl.port || 80),
        hostname: "0.0.0.0",
        signal,
        onListen() {
          logger.info("OAuth server listening");
        },
      },
      (req) => this.handleRequest(req, redirectUrl),
    );
  }

  private async handleRequest(
    req: Request,
    redirectUrl: URL,
  ): Promise<Response> {
    const reqUrl = new URL(req.url);

    // Health check endpoint
    if (reqUrl.pathname === "/healthz") {
      return new Response(JSON.stringify({ status: "ok" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }

    // Auth redirect
    if (reqUrl.pathname === "/" || reqUrl.pathname === "/auth") {
      return this.handleAuthRedirect();
    }

    // Callback
    if (
      reqUrl.pathname === redirectUrl.pathname &&
      reqUrl.searchParams.has("code")
    ) {
      return await this.handleCallback(reqUrl);
    }

    // Token API - protected
    if (reqUrl.pathname === "/api/token") {
      return await this.handleTokenApi(req);
    }

    return new Response("Not Found", { status: 404 });
  }

  private handleAuthRedirect(): Response {
    // Clean expired states
    this.cleanExpiredStates();

    const { url, state } = this.authorizeUsecase.buildAuthorizationUrl();
    this.pendingStates.set(state, {
      state,
      createdAt: Date.now(),
    });

    logger.info("New auth request initiated");

    return new Response(null, {
      status: 302,
      headers: { Location: url },
    });
  }

  private async handleCallback(reqUrl: URL): Promise<Response> {
    const code = reqUrl.searchParams.get("code")!;
    const returnedState = reqUrl.searchParams.get("state");

    // Validate state parameter
    if (!returnedState || !this.pendingStates.has(returnedState)) {
      logger.error("Invalid or missing state parameter in callback");
      return new Response("Security Error: Invalid state parameter", {
        status: 400,
      });
    }

    const pendingState = this.pendingStates.get(returnedState)!;
    this.pendingStates.delete(returnedState);

    // Check TTL
    if (Date.now() - pendingState.createdAt > STATE_TTL_MS) {
      logger.error("State parameter expired");
      return new Response("Security Error: State parameter expired", {
        status: 400,
      });
    }

    try {
      await this.authorizeUsecase.exchangeCodeAndStore(code);
      logger.info("Tokens successfully exchanged and stored");
      return new Response(
        "Authorization successful! Tokens have been stored. You may close this tab.",
        { status: 200 },
      );
    } catch (err) {
      logger.error("Error processing callback", {
        error: err instanceof Error ? err.message : String(err),
      });
      return new Response("Internal Server Error during processing", {
        status: 500,
      });
    }
  }

  private async handleTokenApi(req: Request): Promise<Response> {
    // Simple internal auth header check
    const authHeader = req.headers.get("X-Internal-Auth");
    const expectedToken = Deno.env.get("INTERNAL_AUTH_TOKEN");

    if (expectedToken && authHeader !== expectedToken) {
      return new Response(JSON.stringify({ error: "Unauthorized" }), {
        status: 401,
        headers: { "Content-Type": "application/json" },
      });
    }

    try {
      const tokenData = await this.secretManager.getTokenSecret();

      if (!tokenData) {
        return new Response(
          JSON.stringify({ error: "No token data found" }),
          {
            status: 404,
            headers: { "Content-Type": "application/json" },
          },
        );
      }

      // Return only metadata, not full tokens (unless explicitly internal)
      if (!expectedToken) {
        // No auth configured - return limited info
        return new Response(
          JSON.stringify({
            has_access_token: Boolean(tokenData.access_token),
            has_refresh_token: Boolean(tokenData.refresh_token),
            expires_at: tokenData.expires_at,
            updated_at: tokenData.updated_at,
            token_type: tokenData.token_type,
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        );
      }

      return new Response(JSON.stringify(tokenData), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    } catch (err) {
      logger.error("Failed to serve token API", {
        error: err instanceof Error ? err.message : String(err),
      });
      return new Response(
        JSON.stringify({ error: "Internal Server Error" }),
        {
          status: 500,
          headers: { "Content-Type": "application/json" },
        },
      );
    }
  }

  private cleanExpiredStates(): void {
    const now = Date.now();
    for (const [key, value] of this.pendingStates) {
      if (now - value.createdAt > STATE_TTL_MS) {
        this.pendingStates.delete(key);
      }
    }
  }
}
