import { NextRequest, NextResponse } from 'next/server';

const KRATOS_PUBLIC_URL = process.env.KRATOS_PUBLIC_URL || 'http://kratos-public.alt-auth.svc.cluster.local:4433';

/**
 * Validate current user session
 * GET /api/auth/validate
 */
export async function GET(request: NextRequest) {
  try {
    const response = await fetch(`${KRATOS_PUBLIC_URL}/sessions/whoami`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
      },
      signal: AbortSignal.timeout(8000), // 8秒タイムアウト
    });

    if (response.status === 401) {
      return NextResponse.json(null, { status: 401 });
    }

    if (!response.ok) {
      console.error(`Kratos whoami error: ${response.status} ${response.statusText}`);
      return NextResponse.json(
        { error: 'Session validation failed', code: 'VALIDATION_FAILED' },
        { status: response.status }
      );
    }

    const data = await response.json();
    
    // ULTRATHINK FIX: 防御的プログラミング - Kratosセッションデータの安全な変換
    if (!data || typeof data !== 'object') {
      console.error('Invalid Kratos session response:', data);
      return NextResponse.json(
        { error: 'Invalid session data', code: 'INVALID_SESSION_DATA' },
        { status: 502 }
      );
    }
    
    // Transform Kratos session data to frontend user format
    const user = {
      id: data.identity?.id || null,
      email: data.identity?.traits?.email || null,
      name: data.identity?.traits?.name || null,
      active: Boolean(data.active)
    };
    
    // Forward cookies if Kratos sets any
    const headers = new Headers();
    const setCookie = response.headers.get('set-cookie');
    if (setCookie) {
      headers.set('Set-Cookie', setCookie);
    }
    headers.set('Content-Type', 'application/json');

    return NextResponse.json({
      data: user
    }, {
      status: response.status,
      headers,
    });

  } catch (error) {
    console.error('[USER-VALIDATION] User validation error:', {
      error: error instanceof Error ? error.message : 'Unknown error',
      stack: error instanceof Error ? error.stack : undefined,
      timestamp: new Date().toISOString(),
      kratosUrl: KRATOS_PUBLIC_URL
    });
    
    // ULTRATHINK FIX: タイムアウトエラー処理
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