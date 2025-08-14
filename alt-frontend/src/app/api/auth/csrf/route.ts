import { NextRequest, NextResponse } from 'next/server';

const AUTH_SERVICE_URL = process.env.AUTH_SERVICE_URL || 'http://auth-service.alt-auth.svc.cluster.local:8080';

/**
 * Get CSRF token for secure form submissions
 * POST /api/auth/csrf
 */
export async function POST(request: NextRequest) {
  try {
    // Get CSRF token from auth-service
    const response = await fetch(`${AUTH_SERVICE_URL}/v1/csrf`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
      },
      credentials: 'include',
      signal: AbortSignal.timeout(10000),
    });

    if (!response.ok) {
      console.error(`Auth-service CSRF error: ${response.status} ${response.statusText}`);
      return NextResponse.json(
        { error: 'Failed to get CSRF token from auth-service', code: 'AUTH_CSRF_FAILED' },
        { status: response.status }
      );
    }

    const data = await response.json();

    // Validate auth-service CSRF response
    if (!data || !data.data) {
      console.error('Invalid auth-service CSRF response:', data);
      return NextResponse.json(
        { error: 'Invalid CSRF response format', code: 'INVALID_CSRF_RESPONSE' },
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

    // Return auth-service CSRF response directly
    return NextResponse.json(data, {
      status: 200,
      headers,
    });

  } catch (error) {
    console.error('[CSRF-ROUTE] CSRF token error:', {
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
      { error: 'Failed to get CSRF token', code: 'INTERNAL_ERROR' },
      { status: 500 }
    );
  }
}