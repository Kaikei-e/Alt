// src/lib/server-fetch.ts
export async function serverFetch<T>(endpoint: string): Promise<T> {
  const { cookies, headers } = await import('next/headers');
  const cookieHeader =
    (await cookies()).getAll().map(c => `${c.name}=${c.value}`).join('; ');

  const url = `${process.env.API_URL}${endpoint}`; // 例: http://alt-backend:9000

  const r = await fetch(url, {
    headers: {
      'Content-Type': 'application/json',
      'Cookie': cookieHeader,
      'X-Request-Source': 'SSR',
      'X-Forwarded-For': (await headers()).get('x-forwarded-for') ?? '',
    },
    cache: 'no-store',
    credentials: 'include',
  });

  if (!r.ok) throw new Error(`API ${r.status} for ${endpoint}`);
  return r.json() as Promise<T>;
}