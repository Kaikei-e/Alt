import { NextRequest, NextResponse } from 'next/server';

const AUTH_SERVICE_URL = process.env.AUTH_URL || 'http://auth-service.alt-auth.svc.cluster.local:8080';

/**
 * Complete registration flow with user data
 * POST /api/auth/register/[flowId]
 */
export async function POST(
  request: NextRequest,
  { params }: { params: { flowId: string } }
) {
  try {
    const { flowId } = params;
    const body = await request.text();

    const response = await fetch(`${AUTH_SERVICE_URL}/v1/auth/register/${flowId}`, {
      method: 'POST',
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
    console.error('Registration completion error:', error);
    return NextResponse.json(
      { error: 'Failed to complete registration' },
      { status: 500 }
    );
  }
}