"use server";

import { cookies, headers as nextHeaders } from "next/headers";

import {
  buildBackendIdentityHeaders,
  type BackendIdentityHeaders,
} from "./backend-headers";
import type { User } from "@/types/auth";

interface ValidateResponse {
  ok?: boolean;
  user?: unknown;
  data?: unknown;
  session?: { id?: string | null } | null;
}

const toUser = (payload: unknown) => {
  if (!payload || typeof payload !== "object") return null;
  return payload as Record<string, unknown>;
};

const getOrigin = (headerStore: Headers): string => {
  const proto = headerStore.get("x-forwarded-proto") || "https";
  const host = headerStore.get("host") || "localhost:3000";
  return `${proto}://${host}`;
};

export const getServerSessionHeaders =
  async (): Promise<BackendIdentityHeaders | null> => {
    const cookieStore = await cookies();
    const cookieHeader = cookieStore
      .getAll()
      .map((item) => `${item.name}=${item.value}`)
      .join("; ");

    if (!cookieHeader) {
      return null;
    }

    const headerStore = await nextHeaders();
    const origin = getOrigin(headerStore);

    const response = await fetch(`${origin}/api/fe-auth/validate`, {
      headers: { cookie: cookieHeader },
      cache: "no-store",
    });

    if (!response.ok) {
      return null;
    }

    const data = (await response.json()) as ValidateResponse;
    if (!data.ok) {
      return null;
    }

    const user = (toUser(data.user) || toUser(data.data)) as User | null;

    return buildBackendIdentityHeaders(user, data.session?.id ?? null) ?? null;
  };
