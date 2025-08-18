// app/middleware.ts - ネットワーク無しゲート
import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'

export const config = {
  // 認証系・自己参照・静的は完全除外
  matcher: ['/((?!login|register|self-service|sessions|_next|api|favicon.ico|icon.svg).*)'],
}

export default function middleware(req: NextRequest) {
  const { pathname, search } = req.nextUrl
  
  // ここでネットワーク呼び出しは絶対にしない
  const hasKratos = !!req.cookies.get('ory_kratos_session')?.value

  // ログインページからの往復を防ぐ：ログイン中は素通し
  if (pathname.startsWith('/login') || pathname.startsWith('/register')) {
    return NextResponse.next()
  }

  if (!hasKratos) {
    const url = new URL('/login', req.url)
    // 元の行き先を保持（固定 /dashboard をやめる）
    url.searchParams.set('return_to', pathname + (search || ''))
    return NextResponse.redirect(url)
  }

  // Cookieがあるときは通す。真正性検証は各ページ（SSR）で内部 whoami。
  const res = NextResponse.next()
  res.headers.set('Cache-Control', 'private, no-store, max-age=0')
  return res
}