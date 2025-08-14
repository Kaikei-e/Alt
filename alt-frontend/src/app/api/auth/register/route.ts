import { NextRequest, NextResponse } from 'next/server';

const AUTH_SERVICE_URL = process.env.AUTH_SERVICE_URL || 'http://auth-service.alt-auth.svc.cluster.local:8080';

/**
 * Initiate registration flow
 * POST /api/auth/register
 */
export async function POST(request: NextRequest) {
  try {
    // 🚨 強化されたエラーハンドリング: auth-serviceへのアクセス
    const response = await fetch(`${AUTH_SERVICE_URL}/v1/register`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
      },
      signal: AbortSignal.timeout(10000), // 10秒タイムアウト
    });

    // 🚨 レスポンス検証強化
    if (!response.ok) {
      console.error(`Auth-service registration flow error: ${response.status} ${response.statusText}`);
      
      if (response.status === 400) {
        return NextResponse.json(
          { error: 'Invalid registration data', code: 'INVALID_DATA' },
          { status: 400 }
        );
      }
      
      if (response.status === 409) {
        return NextResponse.json(
          { error: 'User already exists', code: 'USER_EXISTS' },
          { status: 409 }
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
        { error: 'Registration initiation failed', code: 'REGISTRATION_INIT_FAILED' },
        { status: response.status }
      );
    }

    const data = await response.json();
    
    // 🚨 auth-serviceレスポンスデータの検証
    if (!data || !data.data) {
      console.error('Auth-service registration flow response missing required fields:', data);
      return NextResponse.json(
        { error: 'Invalid registration flow response', code: 'INVALID_FLOW' },
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
    
    console.log('[REGISTER-ROUTE] Registration flow initiated successfully:', { flowId: data.data?.id, timestamp: new Date().toISOString() });

    return NextResponse.json(data, {
      status: response.status,
      headers,
    });

  } catch (error) {
    console.error('[REGISTER-ROUTE] Registration initiation error:', {
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