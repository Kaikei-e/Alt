import { NextRequest, NextResponse } from 'next/server';

const KRATOS_PUBLIC_URL = process.env.KRATOS_PUBLIC_URL || 'http://kratos-public.alt-auth.svc.cluster.local:4433';

/**
 * Complete login flow with credentials
 * POST /api/auth/login/[flowId]
 */
export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ flowId: string }> }
) {
  try {
    const { flowId } = await params;
    
    // ULTRATHINK FIX: flowId検証
    if (!flowId || typeof flowId !== 'string' || flowId.trim().length === 0) {
      return NextResponse.json(
        { error: 'Invalid flow ID', code: 'INVALID_FLOW_ID' },
        { status: 400 }
      );
    }
    
    const body = await request.text();

    const response = await fetch(`${KRATOS_PUBLIC_URL}/self-service/login?flow=${encodeURIComponent(flowId)}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
      },
      body,
      signal: AbortSignal.timeout(15000), // 15秒タイムアウト
    });

    // ULTRATHINK FIX: レスポンス検証強化
    if (!response.ok) {
      console.error(`Kratos login completion error: ${response.status} ${response.statusText}`, { flowId });
      
      if (response.status === 400) {
        const errorData = await response.json().catch(() => ({}));
        return NextResponse.json(
          { error: 'Invalid login credentials or expired flow', code: 'LOGIN_FAILED', details: errorData },
          { status: 400 }
        );
      }
      
      if (response.status === 401) {
        return NextResponse.json(
          { error: 'Authentication failed', code: 'AUTH_FAILED' },
          { status: 401 }
        );
      }
      
      return NextResponse.json(
        { error: 'Login service unavailable', code: 'SERVICE_ERROR' },
        { status: 502 }
      );
    }

    const data = await response.json();
    
    // Forward authentication headers
    const headers = new Headers();
    const setCookie = response.headers.get('set-cookie');
    if (setCookie) {
      headers.set('Set-Cookie', setCookie);
    }
    headers.set('Content-Type', 'application/json');

    return NextResponse.json({
      data: data
    }, {
      status: response.status,
      headers,
    });

  } catch (error) {
    console.error('[LOGIN-COMPLETION] Login completion error:', {
      error: error instanceof Error ? error.message : 'Unknown error',
      stack: error instanceof Error ? error.stack : undefined,
      timestamp: new Date().toISOString(),
      kratosUrl: KRATOS_PUBLIC_URL
    });
    
    // ULTRATHINK FIX: タイムアウトエラー処理
    if (error instanceof Error && error.name === 'AbortError') {
      return NextResponse.json(
        { error: 'Login timeout', code: 'TIMEOUT' },
        { status: 408 }
      );
    }
    
    return NextResponse.json(
      { error: 'Failed to complete login', code: 'INTERNAL_ERROR' },
      { status: 500 }
    );
  }
}