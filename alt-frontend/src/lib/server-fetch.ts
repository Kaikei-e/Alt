import { headers, cookies } from 'next/headers'

async function buildCookieHeader(): Promise<string> {
  const hdr = (await headers()).get('cookie')
  if (hdr && hdr.trim().length > 0) return hdr
  const jar = (await cookies()).getAll().map(c => `${c.name}=${c.value}`).join('; ')
  return jar
}

/** SSR専用: 内向き /v1/** へ Cookie を確実に前方転送 */
export async function serverFetch<T>(endpoint: string, init: RequestInit = {}): Promise<T> {
  const cookieHdr = await buildCookieHeader()
  const url = `${process.env.API_URL}${endpoint}` // 例: http://alt-backend.alt-apps.svc.cluster.local:9000

  const res = await fetch(url, {
    ...init,
    headers: {
      'Cookie': cookieHdr,
      'Content-Type': 'application/json',
      ...(init.headers ?? {}),
    },
    cache: 'no-store',                  // 認証状態をキャッシュさせない
  })
  if (!res.ok) throw new Error(`API ${res.status} @ ${endpoint}`)
  return res.json() as Promise<T>
}