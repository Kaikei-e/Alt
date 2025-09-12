// SSR専用：ブラウザCookieを明示転送し、Data Cacheを避ける
export async function serverFetch<T>(endpoint: string): Promise<T> {
  const { headers, cookies } = await import("next/headers");
  const hdr = await headers();
  const cookieHdr =
    hdr.get("cookie") ??
    (await cookies())
      .getAll()
      .map((c) => `${c.name}=${c.value}`)
      .join("; ");

  const url = `${process.env.API_URL}${endpoint}`;
  const res = await fetch(url, {
    headers: {
      Cookie: cookieHdr,
      "Content-Type": "application/json",
      // 受けた x-forwarded-for / proto は backend へも引き継ぐと吉
      "X-Forwarded-For": hdr.get("x-forwarded-for") ?? "",
      "X-Forwarded-Proto": hdr.get("x-forwarded-proto") ?? "https",
    },
    cache: "no-store",
  });

  if (!res.ok) throw new Error(`API ${res.status} for ${endpoint}`);
  return res.json() as Promise<T>;
}
