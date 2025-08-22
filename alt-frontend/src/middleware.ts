// alt-frontend/src/middleware.ts
import { NextRequest, NextResponse } from 'next/server'

const AUTH_GUARD_COOKIE = 'alt_auth_redirect_guard'
const GUARD_TTL = 10 // sec

export const config = {
  matcher: [
    // すべて見るが、除外は中で明示
    '/:path*',
  ],
}

function isBypassPath(url: URL): boolean {
  const p = url.pathname
  return (
    p.startsWith('/auth/') ||
    p.startsWith('/api/') ||
    p.startsWith('/_next/') ||
    p.startsWith('/static/') ||
    p === '/favicon.ico' ||
    p === '/robots.txt'
  )
}

export default async function middleware(req: NextRequest) {
  const url = req.nextUrl
  if (isBypassPath(url)) return NextResponse.next()

  // すでにループガードがある場合は素通し
  if (req.cookies.get(AUTH_GUARD_COOKIE)) {
    return NextResponse.next()
  }

  // whoami で認証状態を確認（TODO.md Step C準拠）
  const proto = req.headers.get('x-forwarded-proto') ?? url.protocol.replace(':','')
  let host = req.headers.get('x-forwarded-host') ?? req.headers.get('host') ?? url.host
  
  // 0.0.0.0や127.0.0.1などの内部アドレスを適切なホスト名に変換
  if (host.startsWith('0.0.0.0:') || host.startsWith('127.0.0.1:') || host.startsWith('localhost:')) {
    host = 'curionoah.com'
  }
  
  const origin = `${proto}://${host}`

  // whoami で認証状態チェック
  try {
    const who = await fetch('https://id.curionoah.com/sessions/whoami', {
      headers: { cookie: req.headers.get('cookie') ?? '' },
      credentials: 'include',
    })
    if (who.ok) return NextResponse.next()
  } catch (error) {
    // ネットワークエラー等はログインに誘導
  }

  // 未認証の場合、ログイン後に戻るURL（現リクエストの絶対URL）
  const returnTo = encodeURIComponent(`${origin}${url.pathname}${url.search}`)
  const loginInit = `https://id.curionoah.com/self-service/login/browser?return_to=${returnTo}`

  const res = NextResponse.redirect(loginInit, { status: 303 }) // 303 See Other

  // 10秒のループガード Cookie をセット（ドメインは親に）
  res.cookies.set({
    name: AUTH_GUARD_COOKIE,
    value: '1',
    maxAge: GUARD_TTL,
    httpOnly: true,
    sameSite: 'lax',
    secure: true,
    path: '/',
    domain: 'curionoah.com',
  })

  return res
}
