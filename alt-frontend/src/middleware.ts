// middleware.ts - ネットワーク無しゲート
import { NextRequest, NextResponse } from 'next/server'
import { securityHeaders } from './config/security'

export const config = {
  // 認証系・自己参照・静的は完全除外
  matcher: ['/((?!login|register|self-service|sessions|_next|api|favicon.ico|icon.svg).*)'],
}

export default function middleware(req: NextRequest) {
  const { pathname, search } = req.nextUrl
  
  // ここでネットワーク呼び出しは絶対にしない
  // TODO.md要件: 堅牢化（通常名 + ホスト名付きクッキーのフォールバック）
  const s1 = req.cookies.get('ory_kratos_session')?.value
  const s2 = req.cookies.get('__Host-ory_kratos_session')?.value
  const hasKratos = !!(s1 ?? s2)

  // ログインページからの往復を防ぐ：ログイン中は素通し
  if (pathname.startsWith('/login') || pathname.startsWith('/register')) {
    const nonce = crypto.randomUUID()
    const res = NextResponse.next()
    const headers = securityHeaders(nonce)
    for (const [k, v] of Object.entries(headers)) res.headers.set(k, v)
    res.headers.set('x-nonce', nonce)
    return res
  }

  if (!hasKratos) {
    const url = new URL('/login', req.url)
    // 元の行き先を保持（固定 /dashboard をやめる）
    url.searchParams.set('return_to', pathname + (search || ''))
    return NextResponse.redirect(url)
  }

  // Cookieがあるときは通す。真正性検証は各ページ（SSR）で内部 whoami。
  const nonce = crypto.randomUUID()
  const res = NextResponse.next()
  const headers = securityHeaders(nonce)
  for (const [k, v] of Object.entries(headers)) res.headers.set(k, v)
  res.headers.set('x-nonce', nonce)
  return res
}
