// src/lib/server-fetch.ts
import { headers } from "next/headers";

export async function serverFetch<T>(
  endpoint: string,
  init: RequestInit = {},
): Promise<T> {
  const h = await headers();
  const cookie = h.get("cookie") ?? "";
  const url = `${process.env.API_URL}${endpoint}`; // e.g. http://alt-backend:9000

  const res = await fetch(url, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      Cookie: cookie,
      ...(init.headers || {}),
    },
    cache: "no-store",
  });
  if (!res.ok) throw new Error(`API ${res.status} for ${endpoint}`);
  return res.json() as Promise<T>;
}
