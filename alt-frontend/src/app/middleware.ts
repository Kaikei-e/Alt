// app/middleware.ts
import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'

export const config = {
  // ← ここで Kratos/静的系を物理的に除外
  matcher: ['/((?!login|register|self-service|sessions|_next|api|favicon.ico).*)'],
}

export async function middleware(req: NextRequest) {
  // ここから先は「保護したいアプリ本体のみに発火」
  // 例：whoamiが200なら通す、401なら /login へ、など簡潔に
  const KRATOS = 'https://id.curionoah.com'
  const who = await fetch(`${KRATOS}/sessions/whoami`, {
    headers: { cookie: req.headers.get('cookie') ?? '' },
    // サーバfetchの個別キャッシュを完全無効化（重要）
    cache: 'no-store',
  })
  if (who.ok) {
    const res = NextResponse.next()
    res.headers.set('Cache-Control', 'private, no-store, max-age=0')
    return res
  }

  const url = new URL('/login', req.url)
  url.searchParams.set('return_to', req.nextUrl.pathname)
  return NextResponse.redirect(url)
}