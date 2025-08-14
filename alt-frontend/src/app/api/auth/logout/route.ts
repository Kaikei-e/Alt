import { NextRequest, NextResponse } from 'next/server';

const AUTH_SERVICE_URL = process.env.AUTH_SERVICE_URL || 'http://auth-service.alt-auth.svc.cluster.local:8080';

/**
 * Logout user and clear session
 * POST /api/auth/logout
 */
export async function POST(request: NextRequest) {
  try {
    // Logout via auth-service
    const response = await fetch(`${AUTH_SERVICE_URL}/v1/auth/logout`, {
      method: 'POST',
      headers: {
        'Accept': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
      },
    });

    if (!response.ok) {
      console.error(`Auth-service logout error: ${response.status} ${response.statusText}`);
      return NextResponse.json(
        { error: 'Failed to logout', code: 'LOGOUT_FAILED' },
        { status: response.status }
      );
    }

    const data = await response.json().catch(() => ({ success: true, message: 'Logout successful' }));

    // Forward headers to clear auth cookies
    const headers = new Headers();
    const setCookie = response.headers.get('set-cookie');
    if (setCookie) {
      headers.set('Set-Cookie', setCookie);
    }
    headers.set('Content-Type', 'application/json');

    return NextResponse.json({
      data: data
    }, {
      status: 200,
      headers,
    });

  } catch (error) {
    console.error('[LOGOUT] Logout error:', {
      error: error instanceof Error ? error.message : 'Unknown error',
      stack: error instanceof Error ? error.stack : undefined,
      timestamp: new Date().toISOString(),
      authServiceUrl: AUTH_SERVICE_URL
    });
    return NextResponse.json(
      { error: 'Failed to logout', code: 'INTERNAL_ERROR' },
      { status: 500 }
    );
  }
}