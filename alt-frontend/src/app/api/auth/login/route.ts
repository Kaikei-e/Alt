import { NextRequest, NextResponse } from 'next/server';

const KRATOS_PUBLIC_URL = process.env.KRATOS_PUBLIC_URL || 'http://kratos-public.alt-auth.svc.cluster.local:4433';

/**
 * Initiate login flow
 * POST /api/auth/login
 */
export async function POST(request: NextRequest) {
  try {
    // 🚨 強化されたエラーハンドリング: Kratosへの直接アクセス
    const response = await fetch(`${KRATOS_PUBLIC_URL}/self-service/login/browser`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
      },
      signal: AbortSignal.timeout(10000), // 10秒タイムアウト
    });

    // 🚨 レスポンス検証強化
    if (!response.ok) {
      console.error(`Kratos login flow error: ${response.status} ${response.statusText}`);
      
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
          { error: 'Kratos service unavailable', code: 'SERVICE_UNAVAILABLE' },
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
    
    // 🚨 Kratosレスポンスデータの検証
    if (!data || !data.id || !data.ui) {
      console.error('Kratos login flow response missing required fields:', data);
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

    return NextResponse.json({
      data: data
    }, {
      status: response.status,
      headers,
    });

  } catch (error) {
    console.error('[LOGIN-ROUTE] Login initiation error:', {
      error: error instanceof Error ? error.message : 'Unknown error',
      stack: error instanceof Error ? error.stack : undefined,
      timestamp: new Date().toISOString(),
      kratosUrl: KRATOS_PUBLIC_URL
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