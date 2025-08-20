// middleware.ts
import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

export function middleware(req: NextRequest) {
  const p = req.nextUrl.pathname;
  const protectedPath = p.startsWith('/desktop/') || p.startsWith('/mobile/');
  if (!protectedPath) return NextResponse.next();

  const hasSession = req.cookies.has('ory_kratos_session');
  if (!hasSession) {
    const login = new URL('/auth/login', req.nextUrl);
    login.searchParams.set('return_to', req.nextUrl.href);
    return NextResponse.redirect(login, 307);
  }
  return NextResponse.next();
}

export const config = { matcher: ['/desktop/:path*','/mobile/:path*'] };
