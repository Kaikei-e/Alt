import { NextRequest, NextResponse } from 'next/server';

const AUTH_SERVICE_URL = process.env.AUTH_URL || 'http://auth-service.alt-auth.svc.cluster.local:8080';

/**
 * Get CSRF token for secure form submissions
 * POST /api/auth/csrf
 */
export async function POST(request: NextRequest) {
  try {
    const response = await fetch(`${AUTH_SERVICE_URL}/v1/auth/csrf`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
      },
    });

    const data = await response.text();
    
    // Forward CSRF and cookie headers
    const headers = new Headers();
    const setCookie = response.headers.get('set-cookie');
    if (setCookie) {
      headers.set('Set-Cookie', setCookie);
    }
    
    const csrfToken = response.headers.get('x-csrf-token');
    if (csrfToken) {
      headers.set('X-CSRF-Token', csrfToken);
    }
    
    headers.set('Content-Type', 'application/json');

    return new NextResponse(data, {
      status: response.status,
      headers,
    });

  } catch (error) {
    console.error('CSRF token error:', error);
    return NextResponse.json(
      { error: 'Failed to get CSRF token' },
      { status: 500 }
    );
  }
}