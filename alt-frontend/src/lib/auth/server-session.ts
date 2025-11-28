"use server";

import { cookies } from "next/headers";

type SessionResult = {
  user: {
    id: string;
    email: string;
    role: string;
  };
  session: {
    id: string;
    active: boolean;
  };
  backendToken: string;
};

/**
 * Resolves server-side session by calling auth-hub directly.
 * This function should only be called from server components or route handlers.
 * Never expose the backendToken to the client.
 */
export async function resolveServerSession(): Promise<SessionResult | null> {
  const cookieStore = await cookies();
  const cookieHeader = cookieStore
    .getAll()
    .map((item) => `${item.name}=${item.value}`)
    .join("; ");

  if (!cookieHeader) {
    return null;
  }

  const authHubUrl =
    process.env.AUTH_HUB_INTERNAL_URL || "http://auth-hub:8888";

  const res = await fetch(`${authHubUrl}/session`, {
    method: "GET",
    headers: {
      cookie: cookieHeader,
    },
    // Internal call, no credentials needed
    cache: "no-store",
  });

  if (!res.ok) {
    return null;
  }

  const data = await res.json();
  const backendToken = res.headers.get("X-Alt-Backend-Token");

  if (!backendToken || !data?.ok || !data.session?.active) {
    return null;
  }

  return {
    user: {
      id: data.user.id,
      email: data.user.email,
      role: data.user.role ?? "user",
    },
    session: {
      id: data.session.id,
      active: true,
    },
    backendToken,
  };
}
