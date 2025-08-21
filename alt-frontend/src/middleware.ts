import { NextResponse, type NextRequest } from 'next/server'

const KRATOS = process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL || 'https://id.curionoah.com'
const PUBLIC = [
  /^\/auth\//, /^\/api\//, /^\/_next\//, /^\/public\//,
  /^\/favicon\.ico$/, /^\/robots\.txt$/, /^\/sitemap\.xml$/, /^\/manifest\.webmanifest$/,
]

// 緊急バイパス用（止血スイッチ）
const MW_BYPASS = process.env.AUTH_MW_DISABLED === '1'

export async function middleware(req: NextRequest) {
  if (MW_BYPASS) return NextResponse.next()

  const { pathname, search } = req.nextUrl
  if (PUBLIC.some(r => r.test(pathname))) return NextResponse.next()

  // ループガード：/auth/login から直帰は素通し
  const referer = req.headers.get('referer') || ''
  if (pathname.startsWith('/auth/login') || referer.includes('/auth/login')) {
    return NextResponse.next()
  }

  // whoami で"確定判定" (Cookie そのまま中継)
  const cookie = req.headers.get('cookie') || ''
  const ok = await fetch(`${KRATOS}/sessions/whoami`, {
    headers: { cookie },
    cache: 'no-store',
    signal: AbortSignal.timeout(3500),
  }).then(r => r.ok).catch(() => false)

  if (ok) return NextResponse.next()

  // 未認証 → 絶対URL return_to で login へ
  const proto = req.headers.get('x-forwarded-proto') || 'https'
  const host  = req.headers.get('x-forwarded-host')  || req.nextUrl.host
  const origin = `${proto}://${host}`

  const login = new URL('/auth/login', origin)
  login.searchParams.set('return_to', new URL(pathname + search, origin).toString())

  return NextResponse.redirect(login, { headers: { 'Cache-Control': 'no-store' } })
}

export const config = { matcher: ['/((?!_next/static|_next/image|favicon.ico).*)'] }
