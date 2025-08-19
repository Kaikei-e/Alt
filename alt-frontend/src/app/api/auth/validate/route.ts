// app/api/auth/validate/route.ts
import { headers } from 'next/headers'
export const dynamic = 'force-dynamic'
export const fetchCache = 'force-no-store'

export async function GET() {
  const cookie = (await headers()).get('cookie') ?? ''
  const r = await fetch(process.env.AUTH_SVC_URL + '/v1/auth/validate', {
    headers: { cookie }, cache: 'no-store'
  })
  return new Response(null, { status: r.status })
}