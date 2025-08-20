// middleware.ts
import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

export function middleware(req: NextRequest) {
  const p = req.nextUrl.pathname;
  const protectedPath = p.startsWith('/desktop') || p.startsWith('/mobile');
  const hasSession = req.cookies.has('ory_kratos_session'); // 後でwhoamiに置換可

  if (protectedPath && !hasSession) {
    const login = new URL('/auth/login', req.nextUrl);
    login.searchParams.set('return_to', req.nextUrl.href);
    return NextResponse.redirect(login);
  }
  return NextResponse.next();
}

export const config = { matcher: ['/desktop/:path*','/mobile/:path*'] };
