// middleware.ts - TODO.md準拠のルート保護実装
import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'

const PROTECTED = [/^\/desktop(\/|$)/, /^\/mobile(\/|$)/]
const IGNORE = [/^\/_next\//, /^\/favicon\.ico$/, /^\/robots\.txt$/, /^\/sitemap\.xml$/, /^\/api\/auth\/validate$/, /^\/auth\//]

export async function middleware(req: NextRequest) {
  const { pathname, origin } = req.nextUrl
  
  // ログ相関ID生成
  const requestId = req.headers.get('x-request-id') || crypto.randomUUID().replace(/-/g, '')
  
  if (IGNORE.some(r => r.test(pathname))) return NextResponse.next()
  if (!PROTECTED.some(r => r.test(pathname))) return NextResponse.next()

  const r = await fetch(new URL('/api/auth/validate', origin), {
    headers: { cookie: req.headers.get('cookie') ?? '' },
    cache: 'no-store',
  })
  if (r.ok) return NextResponse.next()

  const url = new URL('/auth/login', origin)
  url.searchParams.set('return_to', pathname) // Oryの正式名
  return NextResponse.redirect(url)
}

export const config = { matcher: ['/desktop/:path*', '/mobile/:path*'] }
