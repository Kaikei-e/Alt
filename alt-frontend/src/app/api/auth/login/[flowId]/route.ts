import { NextRequest, NextResponse } from 'next/server';

const AUTH_SERVICE_URL = process.env.AUTH_SERVICE_URL || 'http://auth-service.alt-auth.svc.cluster.local:8080';

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

    if (!flowId || typeof flowId !== 'string' || flowId.trim().length === 0) {
      return NextResponse.json(
        { error: 'Invalid flow ID', code: 'INVALID_FLOW_ID' },
        { status: 400 }
      );
    }

    const body = await request.text();

    // CSRF Token二重化 - Body + Header両方で送信
    let csrfToken = '';
    try {
      const bodyData = JSON.parse(body);
      csrfToken = bodyData.csrf_token || '';
    } catch (e) {
      console.warn('Could not parse request body for CSRF token:', e);
    }

    const response = await fetch(`${AUTH_SERVICE_URL}/v1/auth/login/${encodeURIComponent(flowId)}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        'Cookie': request.headers.get('cookie') || '',
        ...(csrfToken && { 'X-CSRF-Token': csrfToken }), // CSRF Header追加
      },
      body,
      credentials: 'include', // SPA用Cookie送信確実化
      signal: AbortSignal.timeout(15000), // 15秒タイムアウト
    });

    // レスポンス検証強化
    if (!response.ok) {
      console.error(`Auth-service login completion error: ${response.status} ${response.statusText}`, { flowId });

      if (response.status === 400) {
        const errorData = await response.json().catch(() => ({}));

        // Flow管理ロジック強化（400エラー時の新Flow初期化）
        console.log(`[FLOW-RECOVERY] 400 error detected for flow ${flowId}, attempting to create new flow`);

        try {
          // Create new login flow automatically
          const newFlowResponse = await fetch(`${AUTH_SERVICE_URL}/v1/auth/login`, {
            method: 'GET',
            headers: {
              'Accept': 'application/json',
              'Cookie': request.headers.get('cookie') || '',
            },
            credentials: 'include',
            signal: AbortSignal.timeout(10000),
          });

          if (newFlowResponse.ok) {
            const newFlowData = await newFlowResponse.json();
            console.log(`[FLOW-RECOVERY] New flow created successfully: ${newFlowData.id}`);

            // Forward new flow cookies
            const recoveryHeaders = new Headers();
            const newSetCookie = newFlowResponse.headers.get('set-cookie');
            if (newSetCookie) {
              recoveryHeaders.set('Set-Cookie', newSetCookie);
            }
            recoveryHeaders.set('Content-Type', 'application/json');

            return NextResponse.json(
              {
                error: 'Flow expired, new flow created',
                code: 'FLOW_EXPIRED_RECOVERED',
                recovery: {
                  new_flow_id: newFlowData.id,
                  new_flow_data: newFlowData
                },
                details: errorData
              },
              {
                status: 400,  // Keep 400 to indicate original issue
                headers: recoveryHeaders
              }
            );
          } else {
            console.error(`[FLOW-RECOVERY] Failed to create new flow: ${newFlowResponse.status}`);
          }
        } catch (recoveryError) {
          console.error('[FLOW-RECOVERY] Flow recovery failed:', recoveryError);
        }

        // Fallback to original error response if recovery fails
        return NextResponse.json(
          { error: 'Invalid login credentials or expired flow', code: 'LOGIN_FAILED', details: errorData },
          { status: 400 }
        );
      }

      if (response.status === 401) {
        return NextResponse.json(
          { error: 'Authentication failed', code: 'AUTH_FAILED' },
          { status: 401 }
        );
      }

      return NextResponse.json(
        { error: 'Login service unavailable', code: 'SERVICE_ERROR' },
        { status: 502 }
      );
    }

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
      authServiceUrl: AUTH_SERVICE_URL
    });

    // タイムアウトエラー処理
    if (error instanceof Error && error.name === 'AbortError') {
      return NextResponse.json(
        { error: 'Login timeout', code: 'TIMEOUT' },
        { status: 408 }
      );
    }

    return NextResponse.json(
      { error: 'Failed to complete login', code: 'INTERNAL_ERROR' },
      { status: 500 }
    );
  }
}