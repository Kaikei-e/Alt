import { NextRequest, NextResponse } from 'next/server';

const AUTH_SERVICE_URL = process.env.AUTH_SERVICE_URL || 'http://auth-service.alt-auth.svc.cluster.local:8080';

/**
 * Get CSRF token for secure form submissions
 * POST /api/auth/csrf
 */
export async function POST(request: NextRequest) {
  try {
    // 🎯 CRITICAL FIX: Check if user has valid session first
    const cookies = request.headers.get('cookie') || '';
    
    console.log(`🔐 [CSRF] Checking session before requesting CSRF token`);
    
    // First, validate if user has an active session
    const sessionCheckResponse = await fetch(`${AUTH_SERVICE_URL}/v1/auth/validate`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
        'Cookie': cookies,
      },
      credentials: 'include',
      signal: AbortSignal.timeout(5000),
    });
    
    if (!sessionCheckResponse.ok) {
      console.log(`🔐 [CSRF] No valid session found (${sessionCheckResponse.status}). CSRF token not needed for public actions.`);
      
      // Return a "no session" response - this is normal for unauthenticated users
      return NextResponse.json(
        { 
          error: 'No active session - CSRF token not required for public actions', 
          code: 'SESSION_NOT_FOUND',
          details: 'Login or register first to get CSRF token for authenticated actions'
        },
        { status: 401 }
      );
    }
    
    console.log(`✅ [CSRF] Valid session found, requesting CSRF token from auth-service`);
    
    // User has valid session, now get CSRF token
    const response = await fetch(`${AUTH_SERVICE_URL}/v1/auth/csrf`, {
      method: 'POST',
      headers: {
        'Accept': 'application/json',
        'Cookie': cookies,
      },
      credentials: 'include',
      signal: AbortSignal.timeout(10000),
    });

    if (!response.ok) {
      console.error(`❌ [CSRF] Auth-service CSRF error: ${response.status} ${response.statusText}`);
      
      // Detailed error logging for debugging
      const errorText = await response.text().catch(() => 'Unable to read error response');
      console.error(`❌ [CSRF] Auth-service error details:`, {
        status: response.status,
        statusText: response.statusText,
        body: errorText,
        url: `${AUTH_SERVICE_URL}/v1/auth/csrf`
      });
      
      return NextResponse.json(
        { 
          error: 'Failed to get CSRF token from auth-service', 
          code: 'AUTH_SERVICE_CSRF_ERROR',
          details: {
            status: response.status,
            statusText: response.statusText,
            authServiceUrl: `${AUTH_SERVICE_URL}/v1/auth/csrf`
          }
        },
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