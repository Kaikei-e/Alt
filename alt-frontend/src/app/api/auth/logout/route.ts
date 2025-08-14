import { NextRequest, NextResponse } from 'next/server';

const KRATOS_PUBLIC_URL = process.env.KRATOS_PUBLIC_URL || 'http://kratos-public.alt-auth.svc.cluster.local:4433';

/**
 * Logout user and clear session
 * POST /api/auth/logout
 */
export async function POST(request: NextRequest) {
  try {
    // First get logout URL from Kratos
    const logoutUrlResponse = await fetch(`${KRATOS_PUBLIC_URL}/self-service/logout/browser`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
      },
    });

    if (!logoutUrlResponse.ok) {
      console.error(`Kratos logout URL error: ${logoutUrlResponse.status} ${logoutUrlResponse.statusText}`);
      return NextResponse.json(
        { error: 'Failed to initiate logout', code: 'LOGOUT_INIT_FAILED' },
        { status: logoutUrlResponse.status }
      );
    }

    const logoutData = await logoutUrlResponse.json();
    const logoutToken = logoutData.logout_token;

    if (!logoutToken) {
      console.error('Logout token missing from Kratos response:', logoutData);
      return NextResponse.json(
        { error: 'Invalid logout response', code: 'INVALID_LOGOUT_RESPONSE' },
        { status: 502 }
      );
    }

    // Execute logout with token
    const response = await fetch(`${KRATOS_PUBLIC_URL}/self-service/logout?token=${logoutToken}`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
      },
    });

    // Forward headers to clear auth cookies
    const headers = new Headers();
    const setCookie = response.headers.get('set-cookie');
    if (setCookie) {
      headers.set('Set-Cookie', setCookie);
    }
    headers.set('Content-Type', 'application/json');

    return NextResponse.json({
      data: { success: true, message: 'Logout successful' }
    }, {
      status: 200,
      headers,
    });

  } catch (error) {
    console.error('[LOGOUT] Logout error:', {
      error: error instanceof Error ? error.message : 'Unknown error',
      stack: error instanceof Error ? error.stack : undefined,
      timestamp: new Date().toISOString(),
      kratosUrl: KRATOS_PUBLIC_URL
    });
    return NextResponse.json(
      { error: 'Failed to logout', code: 'INTERNAL_ERROR' },
      { status: 500 }
    );
  }
}