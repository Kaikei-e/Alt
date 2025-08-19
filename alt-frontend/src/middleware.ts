// middleware.ts
import type { NextRequest } from 'next/server'
import { NextResponse } from 'next/server'

const PROTECTED = [/^\/mobile(?:\/|$)/, /^\/desktop(?:\/|$)/]

export async function middleware(req: NextRequest) {
  const { pathname, origin } = req.nextUrl

  // /auth/* は素通し（ログイン済みでも、ここでは弾かない）
  if (pathname.startsWith('/auth/')) {
    return NextResponse.next()
  }

  // 保護領域でなければ通す
  if (!PROTECTED.some(r => r.test(pathname))) {
    return NextResponse.next()
  }

  // 保護領域のみ、未ログインを /auth/login に飛ばす
  let authRes: Response
  try {
    authRes = await fetch(new URL('/api/auth/validate', origin), {
      headers: { 
        cookie: req.headers.get('cookie') ?? '',
        'x-forwarded-host': req.headers.get('host') ?? '',
        'x-forwarded-proto': req.headers.get('x-forwarded-proto') ?? 'https',
        'x-real-ip': req.headers.get('x-forwarded-for') ?? req.headers.get('x-real-ip') ?? '',
        'user-agent': req.headers.get('user-agent') ?? ''
      },
      cache: 'no-store',
    })
  } catch (error) {
    console.error('Authentication validation failed:', error)
    // Network error, proceed with redirect to login
    const url = new URL('/auth/login', origin)
    url.searchParams.set('return_to', pathname)
    return NextResponse.redirect(url)
  }

  // If authentication successful, proceed
  if (authRes.ok) {
    return NextResponse.next()
  }

  // Authentication failed - create redirect response with session cleanup
  const url = new URL('/auth/login', origin)
  url.searchParams.set('return_to', pathname)
  
  const response = NextResponse.redirect(url)
  
  // Clear potentially stale session cookies on 401 to prevent redirect loops
  if (authRes.status === 401) {
    response.cookies.delete({
      name: 'ory_kratos_session',
      domain: '.curionoah.com',
      path: '/'
    })
    // Also try without domain for broader cleanup
    response.cookies.delete({
      name: 'ory_kratos_session',
      path: '/'
    })
  }
  
  return response
}

export const config = { matcher: ['/desktop/:path*', '/mobile/:path*'] }
