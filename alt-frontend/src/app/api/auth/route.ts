import { NextRequest, NextResponse } from 'next/server';

const AUTH_SERVICE_URL = process.env.AUTH_SERVICE_URL || 'http://auth-service.alt-auth.svc.cluster.local:8080';

/**
 * Generic auth service proxy handler
 * Forwards requests to internal auth-service and preserves cookies/headers
 */
async function proxyToAuthService(
  request: NextRequest,
  endpoint: string,
  method?: string
): Promise<NextResponse> {
  try {
    const url = `${AUTH_SERVICE_URL}${endpoint}`;
    const requestMethod = method || request.method;
    
    // Prepare headers for the proxied request
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'User-Agent': request.headers.get('user-agent') || 'Alt-Frontend-Proxy/1.0',
    };

    // Forward auth-related headers
    const authHeaders = ['authorization', 'cookie', 'x-csrf-token'];
    authHeaders.forEach(header => {
      const value = request.headers.get(header);
      if (value) {
        headers[header] = value;
      }
    });

    // Prepare request body for non-GET methods
    let body: string | undefined;
    if (requestMethod !== 'GET' && requestMethod !== 'HEAD') {
      try {
        body = await request.text();
      } catch (error) {
        console.warn('Failed to read request body:', error);
      }
    }

    // Make the proxied request
    const response = await fetch(url, {
      method: requestMethod,
      headers,
      body,
      // Don't follow redirects to handle them ourselves
      redirect: 'manual',
    });

    // Create response with same status and headers
    const responseHeaders = new Headers();
    
    // Forward important headers from auth service
    const headersToForward = [
      'content-type',
      'set-cookie',
      'cache-control',
      'expires',
      'location',
      'x-csrf-token',
    ];
    
    headersToForward.forEach(header => {
      const value = response.headers.get(header);
      if (value) {
        responseHeaders.set(header, value);
      }
    });

    // Get response body
    const responseBody = await response.text();

    // Create Next.js response
    const nextResponse = new NextResponse(responseBody, {
      status: response.status,
      statusText: response.statusText,
      headers: responseHeaders,
    });

    return nextResponse;

  } catch (error) {
    console.error('Auth proxy error:', error);
    return NextResponse.json(
      { error: 'Authentication service unavailable' },
      { status: 503 }
    );
  }
}

// Health check endpoint
export async function GET(request: NextRequest) {
  return proxyToAuthService(request, '/v1/health');
}

export async function POST(request: NextRequest) {
  return proxyToAuthService(request, '/v1/auth/csrf');
}