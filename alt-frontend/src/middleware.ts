// middleware.ts - TODO.md準拠のAllowlist方式ゲート
import { NextRequest, NextResponse } from 'next/server'
import { securityHeaders } from './config/security'

// TODO.md要件: 保護対象のみを列挙（Allowlist方式）
const PROTECTED_PATHS = [
  /^\/dashboard/,
  /^\/app/,
  /^\/settings/,
  /^\/mobile/,
  /^\/profile/,
  /^\/feeds/,
]

export default function middleware(req: NextRequest) {
  const { pathname, search } = req.nextUrl
  
  // TODO.md要件: 非対象は素通し（/login, /auth, /_next, /self-service などは除外）
  const isProtected = PROTECTED_PATHS.some((regex) => regex.test(pathname))
  if (!isProtected) {
    const nonce = crypto.randomUUID()
    const res = NextResponse.next()
    const headers = securityHeaders(nonce)
    for (const [k, v] of Object.entries(headers)) res.headers.set(k, v)
    res.headers.set('x-nonce', nonce)
    return res
  }

  // TODO.md要件: Cookieの有無で分岐（ネットワーク呼び出しはしない）
  const hasKratosSession = req.cookies.has('ory_kratos_session') || req.cookies.has('__Host-ory_kratos_session')
  
  if (hasKratosSession) {
    // Cookieがあるときは通す。真正性検証は各ページ（SSR）で内部 whoami。
    const nonce = crypto.randomUUID()
    const res = NextResponse.next()
    const headers = securityHeaders(nonce)
    for (const [k, v] of Object.entries(headers)) res.headers.set(k, v)
    res.headers.set('x-nonce', nonce)
    return res
  }

  // 未認証時はログインページにリダイレクト
  const url = new URL('/login', req.url)
  url.searchParams.set('return_to', pathname + (search || ''))
  return NextResponse.redirect(url)
}
