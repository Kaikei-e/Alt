/**
 * Kratos session management for authenticated testing
 */
import type { AuthConfig } from "../config/schema.ts";
import type { SessionCookie } from "../browser/astral.ts";
import { debug, error, info, warn } from "../utils/logger.ts";

interface KratosLoginFlow {
  id: string;
  ui: {
    nodes: Array<{
      attributes: {
        name: string;
        value?: string;
      };
    }>;
  };
}

interface KratosSession {
  id: string;
  identity: {
    id: string;
    traits: {
      email?: string;
    };
  };
}

interface KratosLoginSuccess {
  session?: KratosSession;
  session_token?: string;
}

function maskEmail(email: string): string {
  const at = email.indexOf("@");
  if (at <= 1) return "***";
  return `${email[0]}***${email.slice(at)}`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isKratosLoginFlow(value: unknown): value is KratosLoginFlow {
  if (!isRecord(value) || typeof value.id !== "string") return false;
  if (!isRecord(value.ui) || !Array.isArray(value.ui.nodes)) return false;
  return value.ui.nodes.every((node) => {
    if (!isRecord(node) || !isRecord(node.attributes)) return false;
    return typeof node.attributes.name === "string";
  });
}

function isKratosLoginSuccess(value: unknown): value is KratosLoginSuccess {
  if (!isRecord(value)) return false;
  if (
    value.session_token !== undefined &&
    typeof value.session_token !== "string"
  ) {
    return false;
  }
  if (value.session !== undefined) {
    if (!isRecord(value.session) || typeof value.session.id !== "string") {
      return false;
    }
  }
  return true;
}

function summarizeKratosError(status: number, bodyText: string): string {
  try {
    const parsed = JSON.parse(bodyText) as unknown;
    if (isRecord(parsed)) {
      const err = parsed.error;
      if (isRecord(err) && typeof err.id === "string") {
        return `status=${status} error.id=${err.id}`;
      }
      if (typeof parsed.id === "string") {
        return `status=${status} id=${parsed.id}`;
      }
    }
  } catch {
    // not JSON
  }
  return `status=${status}`;
}

/**
 * Kratos session manager for authenticated testing
 */
export class KratosSessionManager {
  private config: AuthConfig;
  private sessionCookie: SessionCookie | null = null;

  constructor(config: AuthConfig) {
    this.config = config;
  }

  /**
   * Authenticate with Kratos and obtain session cookie
   */
  async authenticate(): Promise<SessionCookie> {
    if (!this.config.enabled) {
      throw new Error("Authentication is disabled");
    }

    const { credentials, kratosUrl } = this.config;

    if (!credentials.email || !credentials.password) {
      throw new Error("Missing authentication credentials (PERF_TEST_EMAIL or PERF_TEST_PASSWORD)");
    }

    info("Authenticating with Kratos", { email: maskEmail(credentials.email) });

    try {
      // Step 1: Create login flow
      debug("Creating login flow");
      const flowResp = await fetch(`${kratosUrl}/self-service/login/api`, {
        method: "GET",
        headers: {
          Accept: "application/json",
        },
        signal: AbortSignal.timeout(10_000),
      });

      if (!flowResp.ok) {
        throw new Error(`Failed to create login flow: ${flowResp.status}`);
      }

      const flowJson: unknown = await flowResp.json();
      if (!isKratosLoginFlow(flowJson)) {
        throw new Error("Invalid Kratos login flow response shape");
      }
      const flow = flowJson;
      debug("Login flow created", { flowId: flow.id });

      // Extract CSRF token
      const csrfNode = flow.ui.nodes.find(
        (n) => n.attributes.name === "csrf_token"
      );
      const csrfToken = csrfNode?.attributes.value || "";

      // Step 2: Submit credentials (always use internal kratosUrl, not the action URL from response)
      const loginUrl = `${kratosUrl}/self-service/login?flow=${flow.id}`;
      debug("Submitting credentials", { url: loginUrl });
      const loginResp = await fetch(loginUrl, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Accept: "application/json",
        },
        body: JSON.stringify({
          method: "password",
          identifier: credentials.email,
          password: credentials.password,
          csrf_token: csrfToken,
        }),
        signal: AbortSignal.timeout(10_000),
      });

      // Read the body once as text; parse as JSON below as needed. A second
      // body read (e.g. .json() then .text()) throws "Body already
      // consumed" and masks the real login failure reason.
      const loginBodyText = await loginResp.text();

      // Check for session cookie in response
      const setCookieHeader = loginResp.headers.get("set-cookie");

      if (loginResp.ok && setCookieHeader) {
        this.sessionCookie = this.parseSessionCookie(setCookieHeader);
        if (this.sessionCookie) {
          info("Authentication successful");
          return this.sessionCookie;
        }
      }

      // If no cookie, check for session in response body
      if (loginResp.ok) {
        let parsed: unknown;
        try {
          parsed = JSON.parse(loginBodyText);
        } catch {
          throw new Error(
            `Login failed: ${summarizeKratosError(loginResp.status, loginBodyText)}`,
          );
        }
        if (!isKratosLoginSuccess(parsed)) {
          throw new Error("Invalid Kratos login success response shape");
        }

        if (parsed.session_token) {
          // API-based session, create a cookie representation
          this.sessionCookie = {
            name: "ory_kratos_session",
            value: parsed.session_token,
            domain: new URL(kratosUrl).hostname,
            path: "/",
            httpOnly: true,
            sameSite: "Lax",
          };
          info("Authentication successful (API token)");
          return this.sessionCookie;
        }
      }

      // Handle error response — never log full body (may echo credentials).
      throw new Error(
        `Login failed: ${summarizeKratosError(loginResp.status, loginBodyText)}`,
      );
    } catch (err) {
      error("Authentication failed", {
        error: String(err),
        kratosUrl,
        email: maskEmail(credentials.email),
      });
      throw err;
    }
  }

  /**
   * Validate current session
   */
  async validateSession(): Promise<boolean> {
    if (!this.sessionCookie) {
      return false;
    }

    try {
      const resp = await fetch(`${this.config.kratosUrl}/sessions/whoami`, {
        headers: {
          Cookie: `${this.sessionCookie.name}=${this.sessionCookie.value}`,
        },
        signal: AbortSignal.timeout(10_000),
      });

      const isValid = resp.ok;
      debug("Session validation", { valid: isValid });
      return isValid;
    } catch {
      warn("Session validation failed");
      return false;
    }
  }

  /**
   * Get session cookies for browser injection
   */
  getCookies(): SessionCookie[] {
    return this.sessionCookie ? [this.sessionCookie] : [];
  }

  /**
   * Check if authenticated
   */
  isAuthenticated(): boolean {
    return this.sessionCookie !== null;
  }

  /**
   * Clear session
   */
  clearSession(): void {
    this.sessionCookie = null;
    debug("Session cleared");
  }

  /**
   * Parse session cookie from Set-Cookie header
   */
  private parseSessionCookie(setCookieHeader: string): SessionCookie | null {
    // Look for ory_kratos_session cookie
    const match = setCookieHeader.match(/ory_kratos_session=([^;]+)/);
    if (!match || match[1] === undefined) {
      return null;
    }

    // Extract domain from kratosUrl
    const domain = new URL(this.config.kratosUrl).hostname;

    return {
      name: "ory_kratos_session",
      value: match[1],
      domain,
      path: "/",
      httpOnly: true,
      sameSite: "Lax",
    };
  }
}

/**
 * Create a Kratos session manager
 */
export function createSessionManager(config: AuthConfig): KratosSessionManager {
  return new KratosSessionManager(config);
}

/**
 * Authenticate and get cookies for testing
 */
export async function getAuthCookies(config: AuthConfig): Promise<SessionCookie[]> {
  if (!config.enabled) {
    return [];
  }

  const manager = createSessionManager(config);
  await manager.authenticate();
  return manager.getCookies();
}
