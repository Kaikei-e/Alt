import { headers, cookies } from 'next/headers'

async function buildCookie(): Promise<string> {
  const hdr = (await headers()).get('cookie')
  if (hdr && hdr.trim()) return hdr
  return (await cookies()).getAll().map(c => `${c.name}=${c.value}`).join('; ')
}

/** SSR専用: 内向き /v1/** を叩く */
export async function serverFetch<T>(endpoint: string, init: RequestInit = {}): Promise<T> {
  const cookie = await buildCookie()
  const url = `${process.env.API_URL}${endpoint}` // 例: http://alt-backend.alt-apps.svc.cluster.local:9000

  const res = await fetch(url, {
    ...init,
    headers: { 'Cookie': cookie, 'Content-Type': 'application/json', ...(init.headers ?? {}) },
    cache: 'no-store',
  })
  if (!res.ok) throw new Error(`API ${res.status} @ ${endpoint}`)
  return res.json() as Promise<T>
}