/**
 * OAuth callback HTTP server handler
 * Security: /api/token requires auth header, state validation with TTL
 */

import type { AuthorizeUsecase } from "../usecase/authorize.ts";
import type { SecretManager } from "../port/secret_manager.ts";
import type { InoreaderCredentials } from "../domain/types.ts";
import { logger } from "../infra/logger.ts";
import { config } from "../infra/config.ts";

interface PendingAuthState {
  state: string;
  createdAt: number;
}

const STATE_TTL_MS = 5 * 60 * 1000; // 5 minutes

/** Constant-time string comparison to avoid timing leaks on auth tokens. */
function timingSafeEqualString(a: string, b: string): boolean {
  const encoder = new TextEncoder();
  const aBytes = encoder.encode(a);
  const bBytes = encoder.encode(b);
  if (aBytes.byteLength !== bBytes.byteLength) {
    return false;
  }
  let diff = 0;
  for (let i = 0; i < aBytes.byteLength; i++) {
    diff |= aBytes[i]! ^ bBytes[i]!;
  }
  return diff === 0;
}

function extractBearerToken(authorizationHeader: string | null): string {
  if (authorizationHeader === null) {
    return "";
  }
  const spaceIndex = authorizationHeader.indexOf(" ");
  if (spaceIndex < 0) {
    return "";
  }
  const scheme = authorizationHeader.substring(0, spaceIndex);
  if (scheme.toLowerCase() !== "bearer") {
    return "";
  }
  return authorizationHeader.substring(spaceIndex + 1);
}

async function extractFormToken(req: Request): Promise<string> {
  try {
    const form = await req.formData();
    const token = form.get("token");
    return typeof token === "string" ? token : "";
  } catch {
    // A body that is not form data is just an unauthenticated request.
    return "";
  }
}

// The browser path for the operator re-auth flow: a navigation cannot attach
// Authorization: Bearer, so /auth serves this form and takes the token from
// the POST body — never from the URL (CWE-598).
const AUTH_TOKEN_FORM = `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>auth-token-manager</title></head>
<body>
<h1>Inoreader re-authorization</h1>
<p>Paste the operator token (INTERNAL_AUTH_TOKEN) to start the OAuth consent flow.</p>
<form method="post" action="/auth">
  <input type="password" name="token" autocomplete="off" autofocus>
  <button type="submit">Authorize</button>
</form>
</body>
</html>
`;

function authTokenPromptResponse(): Response {
  return new Response(AUTH_TOKEN_FORM, {
    status: 401,
    headers: {
      "Content-Type": "text/html; charset=utf-8",
      "Cache-Control": "no-store",
      "Referrer-Policy": "no-referrer",
    },
  });
}

export class OAuthServer {
  private pendingStates = new Map<string, PendingAuthState>();

  constructor(
    private authorizeUsecase: AuthorizeUsecase,
    private secretManager: SecretManager,
    private credentials: InoreaderCredentials,
  ) {}

  start(signal?: AbortSignal): void {
    const redirectUrl = new URL(this.credentials.redirect_uri);

    if (!config.getEnvOrFile("INTERNAL_AUTH_TOKEN")) {
      logger.error(
        "token_api_disabled: INTERNAL_AUTH_TOKEN is not set at startup, /api/token will reject all requests until it is configured",
      );
    }

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

  /**
   * One-shot callback listener for the CLI `authorize` flow.
   * Registers `state`, serves until the callback completes, then shuts down.
   */
  async startOneShot(state: string): Promise<void> {
    this.cleanExpiredStates();
    this.pendingStates.set(state, {
      state,
      createdAt: Date.now(),
    });

    const redirectUrl = new URL(this.credentials.redirect_uri);
    const ac = new AbortController();

    type Outcome = { ok: true } | { ok: false; error: string };
    let settle!: (outcome: Outcome) => void;
    const outcomePromise = new Promise<Outcome>((resolve) => {
      settle = resolve;
    });

    logger.info(
      `Listening for OAuth callback at ${this.credentials.redirect_uri}`,
    );

    const server = Deno.serve(
      {
        port: Number(redirectUrl.port || 80),
        hostname: "0.0.0.0",
        signal: ac.signal,
        onListen() {
          logger.info("One-shot OAuth callback listener ready");
        },
      },
      async (req) => {
        const reqUrl = new URL(req.url);

        if (reqUrl.searchParams.has("error")) {
          const error = reqUrl.searchParams.get("error") ?? "unknown";
          const description = reqUrl.searchParams.get("error_description") ??
            "";
          logger.error("Authorization failed", { error, description });
          settle({ ok: false, error: `${error}: ${description}` });
          return new Response(
            `Authorization failed: ${error} - ${description}`,
            { status: 400 },
          );
        }

        if (
          reqUrl.pathname === redirectUrl.pathname &&
          reqUrl.searchParams.has("code")
        ) {
          const response = await this.handleCallback(reqUrl);
          if (response.status === 200) {
            settle({ ok: true });
          } else {
            const body = await response.clone().text();
            settle({ ok: false, error: body });
          }
          return response;
        }

        return new Response("Invalid OAuth callback request", {
          status: 400,
        });
      },
    );

    try {
      const outcome = await outcomePromise;
      if (!outcome.ok) {
        throw new Error(`Authorization failed: ${outcome.error}`);
      }
    } finally {
      ac.abort();
      await server.finished;
    }
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

    // Auth redirect - protected, otherwise a third party visiting this
    // public port can complete the OAuth flow with their own account and
    // overwrite the stored tokens.
    if (reqUrl.pathname === "/" || reqUrl.pathname === "/auth") {
      return await this.handleAuthRedirect(req);
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

  private async handleAuthRedirect(req: Request): Promise<Response> {
    const expectedToken = config.getEnvOrFile("INTERNAL_AUTH_TOKEN");

    // Fail closed: without a configured secret there is no way to
    // authenticate the caller, so refuse to start an auth flow rather than
    // silently skipping the auth check.
    if (!expectedToken) {
      logger.error(
        "auth_redirect_disabled: INTERNAL_AUTH_TOKEN is not configured, refusing /auth request (fail-closed)",
      );
      return new Response(
        "Auth flow disabled: INTERNAL_AUTH_TOKEN is not configured",
        { status: 503 },
      );
    }

    // CWE-598: never accept INTERNAL_AUTH_TOKEN via ?token= (access logs,
    // browser history, proxies). Scripts send Authorization: Bearer; a
    // browser gets the form and POSTs the token in the request body.
    let providedToken = extractBearerToken(req.headers.get("Authorization"));
    if (providedToken === "" && req.method === "POST") {
      providedToken = await extractFormToken(req);
    }
    if (!(timingSafeEqualString(providedToken, expectedToken))) {
      return authTokenPromptResponse();
    }

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
      headers: {
        Location: url,
        // Defense in depth: do not send this request's URL as Referer to
        // Inoreader's OAuth authorize endpoint.
        "Referrer-Policy": "no-referrer",
      },
    });
  }

  private async handleCallback(reqUrl: URL): Promise<Response> {
    const code = reqUrl.searchParams.get("code");
    const returnedState = reqUrl.searchParams.get("state");

    if (!code) {
      logger.error("Missing authorization code in callback");
      return new Response("Security Error: Missing authorization code", {
        status: 400,
      });
    }

    // Validate state parameter
    if (!returnedState || !this.pendingStates.has(returnedState)) {
      logger.error("Invalid or missing state parameter in callback");
      return new Response("Security Error: Invalid state parameter", {
        status: 400,
      });
    }

    const pendingState = this.pendingStates.get(returnedState);
    if (!pendingState) {
      logger.error("Invalid or missing state parameter in callback");
      return new Response("Security Error: Invalid state parameter", {
        status: 400,
      });
    }
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
    const expectedToken = config.getEnvOrFile("INTERNAL_AUTH_TOKEN");

    // Fail closed: without a configured secret there is no way to
    // authenticate the caller, so refuse to serve tokens rather than
    // silently skipping the auth check.
    if (!expectedToken) {
      logger.error(
        "token_api_disabled: INTERNAL_AUTH_TOKEN is not configured, refusing /api/token request (fail-closed)",
      );
      return new Response(
        JSON.stringify({
          error: "Token API disabled: INTERNAL_AUTH_TOKEN is not configured",
        }),
        {
          status: 503,
          headers: { "Content-Type": "application/json" },
        },
      );
    }

    const authHeader = req.headers.get("X-Internal-Auth") ?? "";
    if (!(timingSafeEqualString(authHeader, expectedToken))) {
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

      // Only access_token/expires_at are needed by consumers; refresh_token
      // never needs to leave this service over the network.
      const { refresh_token: _refresh_token, ...publicTokenData } = tokenData;
      return new Response(JSON.stringify(publicTokenData), {
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
