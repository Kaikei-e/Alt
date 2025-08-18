// app/middleware.ts
import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'

export const config = {
  matcher: ['/((?!login|register|self-service|sessions|_next|api|favicon.ico).*)'],
}

export function middleware(req: NextRequest) {
  const res = NextResponse.next()
  // TODO.md指示: Cache-Control設定でCDN/静的化を防ぐ
  res.headers.set('Cache-Control', 'private, no-store, max-age=0')
  return res
}