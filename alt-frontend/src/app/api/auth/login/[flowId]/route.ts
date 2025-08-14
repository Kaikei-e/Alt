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
    const body = await request.text();

    const response = await fetch(`${KRATOS_PUBLIC_URL}/self-service/login?flow=${flowId}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
      },
      body,
    });

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
    return NextResponse.json(
      { error: 'Failed to complete login', code: 'INTERNAL_ERROR' },
      { status: 500 }
    );
  }
}