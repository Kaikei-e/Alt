import { NextRequest, NextResponse } from 'next/server';

const KRATOS_PUBLIC_URL = process.env.KRATOS_PUBLIC_URL || 'http://kratos-public.alt-auth.svc.cluster.local:4433';

/**
 * Get CSRF token for secure form submissions
 * POST /api/auth/csrf
 */
export async function POST(request: NextRequest) {
  try {
    // Get CSRF token directly from Kratos login flow
    const response = await fetch(`${KRATOS_PUBLIC_URL}/self-service/login/browser`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
      },
      signal: AbortSignal.timeout(10000),
    });

    if (!response.ok) {
      console.error(`Kratos CSRF error: ${response.status} ${response.statusText}`);
      return NextResponse.json(
        { error: 'Failed to get CSRF token from Kratos', code: 'KRATOS_CSRF_FAILED' },
        { status: response.status }
      );
    }

    const data = await response.json();
    
    // Validate Kratos login flow response
    if (!data || !data.id || !data.ui) {
      console.error('Invalid Kratos login flow response:', data);
      return NextResponse.json(
        { error: 'Invalid CSRF response format', code: 'INVALID_CSRF_RESPONSE' },
        { status: 502 }
      );
    }
    
    // Forward cookies from Kratos
    const headers = new Headers();
    const setCookie = response.headers.get('set-cookie');
    if (setCookie) {
      headers.set('Set-Cookie', setCookie);
    }
    headers.set('Content-Type', 'application/json');

    // Return the login flow with CSRF token - ULTRATHINK FIX: 防御的プログラミング
    const csrfNode = data.ui?.nodes?.find((node: any) => node?.attributes?.name === 'csrf_token');
    const csrfToken = csrfNode?.attributes?.value;
    
    return NextResponse.json({
      data: {
        csrf_token: csrfToken,
        flow_id: data.id,
        ui: data.ui
      }
    }, {
      status: 200,
      headers,
    });

  } catch (error) {
    console.error('[CSRF-ROUTE] CSRF token error:', {
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
      { error: 'Failed to get CSRF token', code: 'INTERNAL_ERROR' },
      { status: 500 }
    );
  }
}