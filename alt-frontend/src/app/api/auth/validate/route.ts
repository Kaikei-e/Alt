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
    
    // Transform Kratos session data to frontend user format
    const user = {
      id: data.identity?.id,
      email: data.identity?.traits?.email,
      name: data.identity?.traits?.name,
      active: data.active || false
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
    return NextResponse.json(
      { error: 'Failed to validate user', code: 'INTERNAL_ERROR' },
      { status: 500 }
    );
  }
}