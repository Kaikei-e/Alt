import { NextRequest, NextResponse } from 'next/server';

const AUTH_SERVICE_URL = process.env.AUTH_URL || 'http://auth-service.alt-auth.svc.cluster.local:8080';

/**
 * Get user settings
 * GET /api/auth/settings
 */
export async function GET(request: NextRequest) {
  try {
    const response = await fetch(`${AUTH_SERVICE_URL}/v1/user/settings`, {
      method: 'GET',
      headers: {
        'Cookie': request.headers.get('cookie') || '',
      },
    });

    const data = await response.text();
    
    // Forward cookies if auth service sets any
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
    console.error('Get settings error:', error);
    return NextResponse.json(
      { error: 'Failed to get user settings' },
      { status: 500 }
    );
  }
}

/**
 * Update user settings
 * PUT /api/auth/settings
 */
export async function PUT(request: NextRequest) {
  try {
    const body = await request.text();

    const response = await fetch(`${AUTH_SERVICE_URL}/v1/user/settings`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
        'X-CSRF-Token': request.headers.get('x-csrf-token') || '',
      },
      body,
    });

    const data = await response.text();
    
    // Forward authentication headers
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
    console.error('Settings update error:', error);
    return NextResponse.json(
      { error: 'Failed to update user settings' },
      { status: 500 }
    );
  }
}