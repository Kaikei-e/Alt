// middleware.ts (Edge Runtime - No Node APIs)
import { NextResponse, NextRequest } from 'next/server';

export function middleware(req: NextRequest) {
  const { pathname, searchParams } = req.nextUrl;

  // 1) /auth/* は常に素通し（ログイン済みでも、ここで戻さない）
  if (pathname.startsWith('/auth/')) return NextResponse.next();

  // 2) 保護パスだけ判定
  const isProtected = /^\/(desktop|mobile)(?:\/|$)/.test(pathname);
  if (!isProtected) return NextResponse.next();

  // 3) "存在チェックのみ"でセッション判定（誤って true 既定にしない）
  const hasSession =
    Boolean(req.cookies.get('ory_kratos_session')?.value) ||
    Boolean(req.cookies.get('session_token')?.value);

  if (!hasSession) {
    // 4) 直前に自分が投げたリダイレクトなら二度目を抑止（簡易ガード）
    if (searchParams.get('authredir') === '1') return NextResponse.next();

    const url = req.nextUrl.clone();
    url.pathname = '/auth/login';
    url.searchParams.set('return_to', pathname);
    url.searchParams.set('authredir', '1');

    const res = NextResponse.redirect(url, 307);
    // 5) 307をキャッシュさせない
    res.headers.set('Cache-Control', 'no-store, no-cache, must-revalidate');
    return res;
  }

  return NextResponse.next();
}

export const config = {
  matcher: ['/((?!_next|favicon.ico|robots.txt|sitemap.xml).*)'],
};
