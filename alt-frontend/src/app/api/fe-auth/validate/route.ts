import { headers } from 'next/headers'

export async function GET() {
  const h = await headers()
  const cookie = h.get('cookie') || ''
  const KRATOS_PUBLIC = process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL || 'https://id.curionoah.com'
  const r = await fetch(`${KRATOS_PUBLIC}/sessions/whoami`, {
    headers: { cookie }, cache: 'no-store', signal: AbortSignal.timeout(3000),
  }).catch(() => null)
  return r?.ok
    ? Response.json({ ok: true }, { status: 200 })
    : Response.json({ ok: false }, { status: 401 })
}