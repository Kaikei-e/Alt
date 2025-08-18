import { NextRequest, NextResponse } from 'next/server';

// TODO.md要件: 常時動的レンダリング・キャッシュ無効化
export const dynamic = 'force-dynamic';
export const revalidate = 0;

const AUTH_SERVICE_URL = process.env.AUTH_SERVICE_URL || 'http://auth-service.alt-auth.svc.cluster.local:8080';
let loggedAuthServiceError = false;

/**
 * Validate current user session
 * GET /api/auth/validate
 */
export async function GET(request: NextRequest) {
  try {
    const response = await fetch(`${AUTH_SERVICE_URL}/v1/auth/validate`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
      },
      credentials: 'include', // SPA用Cookie送信確実化
      cache: 'no-store', // TODO.md要件: 常時no-store
      signal: AbortSignal.timeout(8000), // 8秒タイムアウト
    });

    if (response.status === 401) {
      return NextResponse.json(null, { status: 401 });
    }

    if (!response.ok) {
      console.error(`Auth-service validate error: ${response.status} ${response.statusText}`);
      return NextResponse.json(
        { error: 'Session validation failed', code: 'VALIDATION_FAILED' },
        { status: response.status }
      );
    }

    const data = await response.json();

    // 防御的プログラミング - auth-serviceレスポンスデータの安全な変換
    if (!data || typeof data !== 'object') {
      console.error('Invalid auth-service session response:', data);
      return NextResponse.json(
        { error: 'Invalid session data', code: 'INVALID_SESSION_DATA' },
        { status: 502 }
      );
    }

    // Forward cookies from auth-service
    const headers = new Headers();
    const setCookie = response.headers.get('set-cookie');
    if (setCookie) {
      headers.set('Set-Cookie', setCookie);
    }
    headers.set('Content-Type', 'application/json');

    // auth-serviceは既に適切なフォーマットでレスポンスを返すため、そのまま使用
    return NextResponse.json(data, {
      status: response.status,
      headers,
    });

  } catch (error) {
    if (!loggedAuthServiceError) {
      console.warn(
        '[USER-VALIDATION] auth-service fetch failed:',
        error instanceof Error ? error.message : error
      );
      loggedAuthServiceError = true;
    }

    // タイムアウトエラー処理
    if (error instanceof Error && error.name === 'AbortError') {
      return NextResponse.json(
        { error: 'Session validation timeout', code: 'TIMEOUT' },
        { status: 408 }
      );
    }

    return NextResponse.json(
      { error: 'Failed to validate user', code: 'INTERNAL_ERROR' },
      { status: 500 }
    );
  }
}