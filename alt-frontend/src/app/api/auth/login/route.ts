import { NextRequest, NextResponse } from 'next/server';

const AUTH_SERVICE_URL = process.env.AUTH_URL || process.env.AUTH_SERVICE_URL || 'http://localhost:8080';

/**
 * Initiate login flow (Browser-compatible GET handler)
 * GET /api/auth/login?return_to=/
 */
export async function GET(request: NextRequest) {
  const returnTo = request.nextUrl.searchParams.get('return_to') || '/';
  
  try {
    console.log('[LOGIN-ROUTE-GET] Initiating login flow via GET:', { returnTo, timestamp: new Date().toISOString() });
    
    // Auth-serviceの新しいGETエンドポイントを使用（Kratos仕様準拠）
    const response = await fetch(`${AUTH_SERVICE_URL}/v1/auth/login/initiate?return_to=${encodeURIComponent(returnTo)}`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
        'User-Agent': request.headers.get('user-agent') || 'NextJS-Auth-Frontend',
      },
      credentials: 'include', // Kratosセッション用Cookie確保
      signal: AbortSignal.timeout(10000),
    });

    // レスポンス検証（POSTハンドラーと同じロジック）
    if (!response.ok) {
      console.error(`[LOGIN-ROUTE-GET] Auth-service error: ${response.status} ${response.statusText}`);

      if (response.status === 401) {
        return NextResponse.json(
          { error: 'Authentication required', code: 'AUTH_REQUIRED' },
          { status: 401 }
        );
      }

      if (response.status === 403) {
        return NextResponse.json(
          { error: 'Access forbidden', code: 'ACCESS_FORBIDDEN' },
          { status: 403 }
        );
      }

      if (response.status >= 500) {
        return NextResponse.json(
          { error: 'Authentication service unavailable', code: 'SERVICE_UNAVAILABLE' },
          { status: 502 }
        );
      }

      return NextResponse.json(
        { error: 'Login initiation failed', code: 'LOGIN_INIT_FAILED' },
        { status: response.status }
      );
    }

    const data = await response.json();

    // データ検証
    if (!data || !data.id) {
      console.error('[LOGIN-ROUTE-GET] Invalid auth-service response:', data);
      return NextResponse.json(
        { error: 'Invalid login flow response', code: 'INVALID_FLOW' },
        { status: 502 }
      );
    }

    // Cookie転送
    const headers = new Headers();
    const setCookie = response.headers.get('set-cookie');
    if (setCookie) {
      headers.set('Set-Cookie', setCookie);
    }
    headers.set('Content-Type', 'application/json');

    console.log('[LOGIN-ROUTE-GET] Login flow initiated successfully:', { 
      flowId: data.id, 
      returnTo,
      timestamp: new Date().toISOString() 
    });

    // return_toパラメータをレスポンスに含める
    return NextResponse.json({ 
      data: {
        ...data,
        return_to: returnTo
      }
    }, {
      status: response.status,
      headers,
    });

  } catch (error) {
    console.error('[LOGIN-ROUTE-GET] Login initiation error:', {
      error: error instanceof Error ? error.message : 'Unknown error',
      stack: error instanceof Error ? error.stack : undefined,
      returnTo,
      timestamp: new Date().toISOString(),
      authServiceUrl: AUTH_SERVICE_URL
    });

    if (error instanceof Error && error.name === 'AbortError') {
      return NextResponse.json(
        { error: 'Request timeout', code: 'TIMEOUT' },
        { status: 408 }
      );
    }

    return NextResponse.json(
      { error: 'Internal server error', code: 'INTERNAL_ERROR' },
      { status: 500 }
    );
  }
}

/**
 * Initiate login flow (Legacy POST handler)
 * POST /api/auth/login
 */
export async function POST(request: NextRequest) {
  try {
    // 🚨 強化されたエラーハンドリング: auth-serviceへのアクセス
    const response = await fetch(`${AUTH_SERVICE_URL}/v1/auth/login`, {
      method: 'POST',
      headers: {
        'Accept': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
      },
      credentials: 'include', // SPA用Cookie送信確実化
      signal: AbortSignal.timeout(10000), // 10秒タイムアウト
    });

    // 🚨 レスポンス検証強化
    if (!response.ok) {
      console.error(`Auth-service login flow error: ${response.status} ${response.statusText}`);

      if (response.status === 401) {
        return NextResponse.json(
          { error: 'Authentication required', code: 'AUTH_REQUIRED' },
          { status: 401 }
        );
      }

      if (response.status === 403) {
        return NextResponse.json(
          { error: 'Access forbidden', code: 'ACCESS_FORBIDDEN' },
          { status: 403 }
        );
      }

      if (response.status >= 500) {
        return NextResponse.json(
          { error: 'Authentication service unavailable', code: 'SERVICE_UNAVAILABLE' },
          { status: 502 }
        );
      }

      // その他のクライアントエラー
      return NextResponse.json(
        { error: 'Login initiation failed', code: 'LOGIN_INIT_FAILED' },
        { status: response.status }
      );
    }

    const data = await response.json();

    // 🚨 auth-serviceレスポンスデータの検証
    if (!data || !data.id) {
      console.error('Auth-service login flow response missing required fields:', data);
      return NextResponse.json(
        { error: 'Invalid login flow response', code: 'INVALID_FLOW' },
        { status: 502 }
      );
    }

    // Forward Set-Cookie headers
    const headers = new Headers();
    const setCookie = response.headers.get('set-cookie');
    if (setCookie) {
      headers.set('Set-Cookie', setCookie);
    }
    headers.set('Content-Type', 'application/json');

    console.log('[LOGIN-ROUTE] Login flow initiated successfully:', { flowId: data.id, timestamp: new Date().toISOString() });

    // Wrap response in expected format
    return NextResponse.json({ data }, {
      status: response.status,
      headers,
    });

  } catch (error) {
    console.error('[LOGIN-ROUTE] Login initiation error:', {
      error: error instanceof Error ? error.message : 'Unknown error',
      stack: error instanceof Error ? error.stack : undefined,
      timestamp: new Date().toISOString(),
      authServiceUrl: AUTH_SERVICE_URL
    });

    if (error instanceof Error && error.name === 'AbortError') {
      return NextResponse.json(
        { error: 'Request timeout', code: 'TIMEOUT' },
        { status: 408 }
      );
    }

    return NextResponse.json(
      { error: 'Internal server error', code: 'INTERNAL_ERROR' },
      { status: 500 }
    );
  }
}