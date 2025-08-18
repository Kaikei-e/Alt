// src/middleware.ts
import { NextResponse, NextRequest } from 'next/server'

// ミドルウェア本体：Cookieが無ければ /auth/login?return_to=<元URL> へ
export function middleware(req: NextRequest) {
  const has =
    req.cookies.has('ory_kratos_session') ||
    req.cookies.has('__Host-ory_kratos_session')

  if (has) return NextResponse.next()

  const url = req.nextUrl.clone()
  const back = url.pathname + (url.search || '')
  url.pathname = '/auth/login'
  url.search = '' // ここから付け直す
  url.searchParams.set('return_to', back)
  return NextResponse.redirect(url)
}

// ここが超重要：/mobile/* と /desktop/* のみで実行
export const config = {
  matcher: ['/mobile/:path*', '/desktop/:path*'],
}
