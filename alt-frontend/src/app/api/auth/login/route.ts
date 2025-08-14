import { NextRequest, NextResponse } from 'next/server';

const AUTH_SERVICE_URL = process.env.AUTH_URL || 'http://auth-service.alt-auth.svc.cluster.local:8080';

/**
 * Initiate login flow
 * POST /api/auth/login
 */
export async function POST(request: NextRequest) {
  try {
    const response = await fetch(`${AUTH_SERVICE_URL}/v1/auth/login`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
      },
    });

    const data = await response.text();
    
    // Forward Set-Cookie headers
    const headers = new Headers();
    const setCookie = response.headers.get('set-cookie');
    if (setCookie) {
      headers.set('Set-Cookie', setCookie);
    }
    headers.set('Content-Type', 'application/json');

    return new NextResponse(data, {
      status: response.status,
      headers,
    });

  } catch (error) {
    console.error('Login initiation error:', error);
    return NextResponse.json(
      { error: 'Failed to initiate login' },
      { status: 500 }
    );
  }
}