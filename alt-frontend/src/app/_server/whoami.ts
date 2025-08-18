// app/_server/whoami.ts
import { cookies } from 'next/headers'
export const dynamic = 'force-dynamic'

export async function whoami() {
  const c = await cookies()
  return fetch(process.env.KRATOS_PUBLIC_URL + '/sessions/whoami', {
    headers: { cookie: c.toString(), accept: 'application/json' },
    cache: 'no-store',
  })
}