// middleware.ts
import { NextResponse, type NextRequest } from 'next/server'

const KRATOS_PUBLIC = process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL || 'https://id.curionoah.com'

// 公開パスは完全除外（ここに /auth/ と /api/ を含める）
const PUBLIC = [
  /^\/auth\//, /^\/api\//, /^\/_next\//, /^\/public\//,
  /^\/favicon\.ico$/, /^\/robots\.txt$/, /^\/sitemap\.xml$/, /^\/manifest\.webmanifest$/
]

export async function middleware(req: NextRequest) {
  const { pathname, search } = req.nextUrl

  if (PUBLIC.some(r => r.test(pathname))) return NextResponse.next()

  // ループガード：直前が /auth/login なら素通し
  const referer = req.headers.get('referer') || ''
  if (pathname.startsWith('/auth/login') || referer.includes('/auth/login')) {
    return NextResponse.next()
  }

  // whoami で認証確定（Cookieはそのまま転送）
  const cookieHeader = req.headers.get('cookie') || ''
  const ok = await fetch(`${KRATOS_PUBLIC}/sessions/whoami`, {
    headers: { cookie: cookieHeader },
    cache: 'no-store',
    signal: AbortSignal.timeout(3500),
  }).then(r => r.ok).catch(() => false)

  if (ok) return NextResponse.next()

  // 未認証 → /auth/login?return_to=ABSOLUTE
  const proto = req.headers.get('x-forwarded-proto') || 'https'
  const host  = req.headers.get('x-forwarded-host') || req.nextUrl.host
  const origin = `${proto}://${host}`
  const returnTo = new URL(pathname + search, origin).toString()

  const login = new URL('/auth/login', origin)
  login.searchParams.set('return_to', returnTo)

  return NextResponse.redirect(login, { headers: { 'Cache-Control': 'no-store' } })
}

export const config = { matcher: ['/((?!_next/static|_next/image|favicon.ico).*)'] }
