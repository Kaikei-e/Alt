import { headers } from 'next/headers'

export async function GET() {
  const cookie = (await headers()).get('cookie') ?? ''
  const KRATOS = process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL || 'https://id.curionoah.com'

  try {
    const r = await fetch(`${KRATOS}/sessions/whoami`, {
      headers: { cookie },
      cache: 'no-store',
      signal: AbortSignal.timeout(3000),
    })
    if (!r.ok) return Response.json({ ok: false }, { status: 401 })
    return Response.json({ ok: true }, { status: 200 })
  } catch (e: any) {
    return Response.json({ ok: false, error: e?.message }, { status: 502 })
  }
}