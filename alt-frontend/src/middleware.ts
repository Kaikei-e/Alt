// middleware.ts
import { NextRequest, NextResponse } from 'next/server'
import { securityHeaders } from './config/security'

export function middleware(req: NextRequest) {
  const { pathname } = req.nextUrl

  // ---- 1) HTML以外は素通り（RSC/_next/API/静的）----
  const isHtml =
    req.headers.get('accept')?.includes('text/html') &&
    !pathname.startsWith('/api') &&
    !pathname.startsWith('/_next') &&
    !/\.(png|jpe?g|webp|avif|svg|ico|css|js|map|txt)$/i.test(pathname) &&
    pathname !== '/manifest.json' &&
    pathname !== '/manifest.webmanifest'
  if (!isHtml) return NextResponse.next()

  // ---- 2) 何もリダイレクトしない。CSP等ヘッダーだけ付与 ----
  const nonce = crypto.randomUUID()
  const res = NextResponse.next()
  const headers = securityHeaders(nonce)
  for (const [k, v] of Object.entries(headers)) res.headers.set(k, v)
  res.headers.set('x-nonce', nonce)
  return res
}

export const config = {
  matcher: ['/((?!login|register|self-service|sessions|_next|api|favicon.ico).*)'],
}
