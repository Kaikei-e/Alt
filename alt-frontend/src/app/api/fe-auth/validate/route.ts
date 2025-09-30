import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { Configuration, FrontendApi, type Session } from "@ory/client";
import type { User } from "@/types/auth";

import { isUuid } from "@/lib/auth/backend-headers";

const CACHE_CONTROL_NO_STORE = "no-store";

const asUserRole = (value: unknown): User["role"] => {
  if (typeof value !== "string") {
    return "user";
  }

  const normalized = value.toLowerCase();

  switch (normalized) {
    case "admin":
      return "admin";
    case "readonly":
      return "readonly";
    case "tenant_admin":
    case "tenant-admin":
    case "tenantadmin":
      return "tenant_admin";
    default:
      return "user";
  }
};

const pickString = (
  source: Record<string, unknown>,
  ...keys: string[]
): string | undefined => {
  for (const key of keys) {
    const value = source[key];
    if (typeof value === "string" && value.trim()) {
      return value.trim();
    }
  }
  return undefined;
};

const extractNestedString = (
  source: Record<string, unknown>,
  keys: string[],
): string | undefined => {
  for (const key of keys) {
    const value = source[key];
    if (typeof value === "string" && value.trim()) {
      return value.trim();
    }

    if (value && typeof value === "object") {
      const nested = value as Record<string, unknown>;
      const nestedCandidate = extractNestedString(nested, [
        "id",
        "uuid",
        "value",
      ]);
      if (nestedCandidate) {
        return nestedCandidate;
      }
    }
  }

  return undefined;
};

const resolveTenantId = (
  identity: Session["identity"],
  merged: Record<string, unknown>,
): string => {
  const candidates = [
    extractNestedString(merged, [
      "tenant_id",
      "tenantId",
      "tenantID",
      "tenant",
    ]),
    extractNestedString(merged, [
      "organization_id",
      "organizationId",
      "org_id",
      "org",
    ]),
    extractNestedString(merged, ["workspace_id", "workspaceId"]),
  ].filter(Boolean) as string[];

  for (const candidate of candidates) {
    if (isUuid(candidate)) {
      return candidate;
    }
  }

  if (
    identity?.metadata_public &&
    typeof identity.metadata_public === "object"
  ) {
    const nestedCandidate = extractNestedString(
      identity.metadata_public as Record<string, unknown>,
      ["tenant_id", "tenant", "id"],
    );
    if (nestedCandidate && isUuid(nestedCandidate)) {
      return nestedCandidate;
    }
  }

  if (identity?.traits && typeof identity.traits === "object") {
    const nestedCandidate = extractNestedString(
      identity.traits as Record<string, unknown>,
      ["tenant_id", "tenant", "id"],
    );
    if (nestedCandidate && isUuid(nestedCandidate)) {
      return nestedCandidate;
    }
  }

  // Fallback: use the identity ID as tenant when no explicit mapping exists.
  if (identity?.id && isUuid(identity.id)) {
    return identity.id;
  }

  const fallbackTenant = (process.env.DEFAULT_TENANT_ID ?? "").trim();
  if (isUuid(fallbackTenant)) {
    return fallbackTenant;
  }

  throw new Error("Tenant identifier missing from Kratos identity metadata");
};

const sessionToUser = (session: Session): User => {
  const identity = session.identity;
  if (!identity) {
    throw new Error("Session response did not include identity information");
  }

  const traits = (identity.traits ?? {}) as Record<string, unknown>;
  const metadataPublic = (identity.metadata_public ?? {}) as Record<
    string,
    unknown
  >;
  const merged = { ...metadataPublic, ...traits };

  const email =
    pickString(merged, "email", "email_address") ||
    identity.verifiable_addresses?.[0]?.value ||
    "";
  const name = pickString(merged, "name", "full_name", "display_name");
  const role = asUserRole(pickString(merged, "role", "user_role", "role_id"));
  const tenantId = resolveTenantId(identity, merged);

  const createdAt =
    identity.created_at ?? session.issued_at ?? new Date().toISOString();
  const lastLoginAt =
    session.authenticated_at ?? session.issued_at ?? undefined;

  const user: User = {
    id: identity.id,
    tenantId,
    email,
    role,
    createdAt,
  };

  if (name) {
    user.name = name;
  }

  if (lastLoginAt) {
    user.lastLoginAt = lastLoginAt;
  }

  if (
    identity.metadata_public &&
    typeof identity.metadata_public === "object"
  ) {
    const preferences = (identity.metadata_public as Record<string, unknown>)
      .preferences;
    if (preferences && typeof preferences === "object") {
      user.preferences = preferences as User["preferences"];
    }
  }

  return user;
};

// Simplified: Use single Kratos internal URL (Ory best practice)
const getKratosUrl = (): string => {
  const kratosUrl = process.env.KRATOS_INTERNAL_URL;
  if (!kratosUrl) {
    throw new Error("KRATOS_INTERNAL_URL environment variable is not set");
  }
  return kratosUrl.replace(/\/$/, "");
};

// Single cached client instance
let oryClient: FrontendApi | null = null;

const getOryClient = (): FrontendApi => {
  if (!oryClient) {
    const baseUrl = getKratosUrl();
    oryClient = new FrontendApi(
      new Configuration({
        basePath: baseUrl,
        baseOptions: {
          withCredentials: true,
          timeout: 5000,
        },
      }),
    );
  }
  return oryClient;
};

export async function GET() {
  const headerStore = await headers();
  const cookieHeader = headerStore.get("cookie");

  if (!cookieHeader || !cookieHeader.trim()) {
    const res = NextResponse.json(
      { ok: false, error: "missing_session_cookie" },
      { status: 401 },
    );
    res.headers.set("cache-control", CACHE_CONTROL_NO_STORE);
    return res;
  }

  try {
    const client = getOryClient();
    // Call Kratos toSession endpoint with cookies
    // Note: withCredentials is not needed as we're explicitly passing cookies in headers
    const { data: session } = await client.toSession(undefined, {
      headers: {
        cookie: cookieHeader,
        // Ensure we request JSON response
        Accept: "application/json",
      },
    });

    const user = sessionToUser(session);
    const res = NextResponse.json({ ok: true, session, user }, { status: 200 });
    res.headers.set("cache-control", CACHE_CONTROL_NO_STORE);
    return res;
  } catch (error: unknown) {
    const status =
      (error as { response?: { status?: number } }).response?.status ??
      (error as { status?: number }).status ??
      null;

    // Unauthorized or forbidden - session invalid or expired
    if (status === 401 || status === 403) {
      const res = NextResponse.json(
        {
          ok: false,
          error: "unauthorized",
          detail: "Session invalid or expired",
        },
        { status: 401 },
      );
      res.headers.set("cache-control", CACHE_CONTROL_NO_STORE);
      return res;
    }

    // Other errors (network, Kratos unavailable, etc.)
    const message = error instanceof Error ? error.message : "unknown_error";
    console.error("Kratos session validation error:", {
      error,
      status,
      message,
    });
    const res = NextResponse.json(
      { ok: false, error: "kratos_whoami_error", message, status },
      { status: 502 },
    );
    res.headers.set("cache-control", CACHE_CONTROL_NO_STORE);
    return res;
  }
}
