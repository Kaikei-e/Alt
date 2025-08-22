// src/middleware.ts
import { NextResponse, NextRequest } from 'next/server';

const PASS = [
  /^\/auth(\/|$)/, /^\/api(\/|$)/,
  /^\/_next\//, /^\/favicon\.ico$/, /^\/robots\.txt$/, /^\/sitemap\.xml$/
];

export default async function middleware(req: NextRequest) {
  const { pathname, search } = req.nextUrl;
  const resHeaders = new Headers();

  if (PASS.some(r => r.test(pathname))) return NextResponse.next();

  // ループガード：短命クッキー方式（次リクエストにも残る）
  const loop = req.cookies.get('__authredir');
  if (loop?.value === '1') {
    const res = NextResponse.next();
    res.cookies.delete('__authredir');
    return res;
  }

  // デバッグ: 環境変数の実際の値を確認
  const whoamiBase = process.env.KRATOS_INTERNAL_URL ?? '(unset)';
  const whoamiURL = `${whoamiBase}/sessions/whoami`;
  
  // デバッグヘッダーを設定
  resHeaders.set('x-auth-debug-whoami', whoamiURL);
  resHeaders.set('x-auth-debug-kratos-internal', whoamiBase);
  resHeaders.set('x-auth-debug-timestamp', new Date().toISOString());

  // デバッグ: コンソールログ出力
  console.log(`[Middleware] Path: ${pathname}, Attempting whoami: ${whoamiURL}`);

  // whoami はクラスタ内の Kratos Public Service へ
  const res = await fetch(whoamiURL, {
    headers: { cookie: req.headers.get('cookie') ?? '' },
    credentials: 'include',
  }).catch(e => {
    console.error(`[Middleware] whoami failed:`, e);
    return { ok: false, status: 599 };
  });

  // デバッグ: whoami レスポンス状態を記録
  console.log(`[Middleware] whoami response status: ${res.status}`);
  resHeaders.set('x-auth-debug-whoami-status', String(res.status));

  if (res.ok) return NextResponse.next({ headers: resHeaders });

  // 未ログイン → 外部の login/browser へ絶対URLで
  const proto = req.headers.get('x-forwarded-proto') ?? 'https';
  const host  = req.headers.get('x-forwarded-host') ?? req.headers.get('host');
  const returnTo = `${proto}://${host}${pathname}${search}`;

  const loginBase = process.env.KRATOS_PUBLIC_URL ?? '(unset)';
  const login = new URL('/self-service/login/browser', loginBase);
  login.searchParams.set('return_to', returnTo);

  // デバッグ: リダイレクト情報をログ出力
  console.log(`[Middleware] Redirecting to login: ${login.toString()}`);
  console.log(`[Middleware] Return to: ${returnTo}`);

  // デバッグヘッダーを追加
  resHeaders.set('x-auth-debug-login', login.toString());
  resHeaders.set('x-auth-debug-return-to', returnTo);
  resHeaders.set('x-auth-debug-kratos-public', loginBase);

  const redirect = NextResponse.redirect(login.toString(), 302);
  redirect.cookies.set('__authredir', '1', { maxAge: 3, path: '/' }); // 3秒だけ有効
  
  // デバッグヘッダーをリダイレクトレスポンスに適用
  for (const [key, value] of resHeaders.entries()) {
    redirect.headers.set(key, value);
  }
  
  return redirect;
}

export const config = {
  matcher: ['/((?!_next|.*\\.(?:png|jpg|jpeg|svg|gif|ico|webp|css|js|map)).*)']
};
