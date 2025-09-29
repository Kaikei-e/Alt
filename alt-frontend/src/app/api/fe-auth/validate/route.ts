import { headers } from "next/headers";
import { NextResponse } from "next/server";
import type { Session } from "@ory/client";
import type { User } from "@/types/auth";

const CACHE_CONTROL_NO_STORE = "no-store";

const asUserRole = (value: unknown): User["role"] => {
  if (value === "admin" || value === "readonly" || value === "user") {
    return value;
  }
  return "user";
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

const sessionToUser = (session: Session): User => {
  const identity = session.identity;
  if (!identity) {
    throw new Error("Session response did not include identity information");
  }

  const traits = (identity.traits ?? {}) as Record<string, unknown>;
  const metadataPublic = (identity.metadata_public ?? {}) as Record<string, unknown>;
  const merged = { ...metadataPublic, ...traits };

  const email =
    pickString(merged, "email", "email_address") ||
    identity.verifiable_addresses?.[0]?.value ||
    "";
  const name = pickString(merged, "name", "full_name", "display_name");
  const role = asUserRole(pickString(merged, "role", "user_role"));
  const tenantId =
    pickString(merged, "tenantId", "tenant_id", "org_id") || "default";

  const createdAt = identity.created_at ?? session.issued_at ?? new Date().toISOString();
  const lastLoginAt = session.authenticated_at ?? session.issued_at ?? undefined;

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

  if (identity.metadata_public && typeof identity.metadata_public === "object") {
    const preferences = (identity.metadata_public as Record<string, unknown>).preferences;
    if (preferences && typeof preferences === "object") {
      user.preferences = preferences as User["preferences"];
    }
  }

  return user;
};

const resolveKratosBaseUrl = (origin: string): string => {
  const configured =
    process.env.KRATOS_INTERNAL_URL ||
    process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL;
  const base = configured || `${origin.replace(/\/$/, "")}/ory`;
  return base.replace(/\/$/, "");
};

export async function GET() {
  const headerStore = headers();
  const cookieHeader = headerStore.get("cookie");

  if (!cookieHeader || !cookieHeader.trim()) {
    const res = NextResponse.json(
      { ok: false, error: "missing_session_cookie" },
      { status: 401 },
    );
    res.headers.set("cache-control", CACHE_CONTROL_NO_STORE);
    return res;
  }

  const proto = headerStore.get("x-forwarded-proto") || "https";
  const host = headerStore.get("host") || "localhost:3000";
  const origin = `${proto}://${host}`;
  const kratosBaseUrl = resolveKratosBaseUrl(origin);

  try {
    const response = await fetch(`${kratosBaseUrl}/sessions/whoami`, {
      headers: { cookie: cookieHeader },
      cache: "no-store",
      credentials: "include",
      signal: AbortSignal.timeout(3000),
    });

    if (response.status === 401) {
      const res = NextResponse.json({ ok: false, error: "unauthorized" }, {
        status: 401,
      });
      res.headers.set("cache-control", CACHE_CONTROL_NO_STORE);
      return res;
    }

    if (!response.ok) {
      const errorPayload = await response.text().catch(() => "");
      const res = NextResponse.json(
        {
          ok: false,
          error: "kratos_whoami_failed",
          details: errorPayload,
        },
        { status: 502 },
      );
      res.headers.set("cache-control", CACHE_CONTROL_NO_STORE);
      return res;
    }

    const session = (await response.json()) as Session;
    if (!session || typeof session !== "object") {
      const res = NextResponse.json(
        { ok: false, error: "invalid_session_response" },
        { status: 502 },
      );
      res.headers.set("cache-control", CACHE_CONTROL_NO_STORE);
      return res;
    }

    const user = sessionToUser(session);
    const res = NextResponse.json(
      { ok: true, session, user },
      { status: 200 },
    );
    res.headers.set("cache-control", CACHE_CONTROL_NO_STORE);
    return res;
  } catch (error: unknown) {
    const message = error instanceof Error ? error.message : "unknown_error";
    const res = NextResponse.json(
      { ok: false, error: "kratos_whoami_error", message },
      { status: 502 },
    );
    res.headers.set("cache-control", CACHE_CONTROL_NO_STORE);
    return res;
  }
}
