// middleware.ts
import { NextRequest, NextResponse } from 'next/server'
import { securityHeaders } from './config/security'

export function middleware(req: NextRequest) {
  const url = req.nextUrl
  const { pathname } = url

  // 1) /mobile および /mobile/* を /desktop 側へ
  if (/^\/mobile(?:\/|$)/.test(pathname)) {
    const dest = url.clone()
    dest.pathname = pathname.replace(/^\/mobile(\/?)/, '/desktop$1')

    // 検証〜安定化期は 307 + no-store（キャッシュ回避）
    const res = NextResponse.redirect(dest, 307)
    res.headers.set('Cache-Control', 'no-store, no-cache, must-revalidate')
    return res
  }

  // 2) HTML 以外は何もしない（RSC/API/静的は素通り）
  const isHtml =
    req.headers.get('accept')?.includes('text/html') &&
    !pathname.startsWith('/api') &&
    !pathname.startsWith('/_next') &&
    !/\.(png|jpe?g|webp|avif|svg|ico|css|js|map|txt)$/i.test(pathname) &&
    pathname !== '/manifest.json' &&
    pathname !== '/manifest.webmanifest'

  if (!isHtml) return NextResponse.next()

  // 3) CSP をレスポンスにだけ設定（Edge で Buffer 不要）
  const nonce = crypto.randomUUID()
  const res = NextResponse.next()
  const headers = securityHeaders(nonce)

  for (const [k, v] of Object.entries(headers)) res.headers.set(k, v)
  res.headers.set('x-nonce', nonce)

  return res
}

export const config = {
  matcher: [
    // /mobile を確実に捕捉（RSC/Fetch も含めて寄せる）
    '/mobile/:path*',
    // 主要なHTMLルート（Next.js 15恒久対応: 明示的パス指定）
    '/',
    '/desktop/:path*',
    '/test/:path*',
  ],
}
