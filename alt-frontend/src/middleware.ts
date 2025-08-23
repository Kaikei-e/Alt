// src/middleware.ts
import { NextRequest, NextResponse } from 'next/server'

const PUBLIC = [
  /^\/$/, /^\/auth(\/|$)/, /^\/api(\/|$)/, /^\/_next(\/|$)/, /^\/static(\/|$)/, /^\/favicon\.ico$/
]

export function middleware(req: NextRequest) {
  const { pathname, search } = req.nextUrl
  if (PUBLIC.some(r => r.test(pathname))) return NextResponse.next()

  // Edgeでは Cookie 有無のみ判定（whoami は呼ばない）
  if (req.cookies.get('ory_kratos_session')) return NextResponse.next()

  // 多重リダイレクト抑止（10s）
  if (req.cookies.get('alt_auth_redirect_guard')) return NextResponse.next()

  const loginInit = new URL(`/ory/self-service/login/browser`, req.url)
  const appOrigin = process.env.NEXT_PUBLIC_APP_ORIGIN || req.nextUrl.origin
  loginInit.searchParams.set('return_to', `${appOrigin}${pathname}${search}`)

  const res = NextResponse.redirect(loginInit, 303)
  res.cookies.set('alt_auth_redirect_guard', '1', {
    maxAge: 10, httpOnly: true, secure: true, sameSite: 'lax', path: '/'
  })
  return res
}

export const config = { matcher: ['/((?!.*\\.).*)'] }
