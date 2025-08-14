import { NextRequest, NextResponse } from 'next/server';

const AUTH_SERVICE_URL = process.env.AUTH_SERVICE_URL || 'http://auth-service.alt-auth.svc.cluster.local:8080';

/**
 * Complete registration flow with user data  
 * POST /api/auth/register/[flowId]
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

    const response = await fetch(`${AUTH_SERVICE_URL}/v1/auth/register/${encodeURIComponent(flowId)}`, {
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
      console.error(`Auth-service registration completion error: ${response.status} ${response.statusText}`, { flowId });

      if (response.status === 400) {
        const errorData = await response.json().catch(() => ({}));

        // Flow管理ロジック強化（400エラー時の新Flow初期化）
        console.log(`[REGISTRATION-RECOVERY] 400 error detected for flow ${flowId}, attempting to create new flow`);

        try {
          // Create new registration flow automatically
          const newFlowResponse = await fetch(`${AUTH_SERVICE_URL}/v1/auth/register`, {
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
            console.log(`[REGISTRATION-RECOVERY] New registration flow created successfully: ${newFlowData.id}`);

            // Forward new flow cookies
            const recoveryHeaders = new Headers();
            const newSetCookie = newFlowResponse.headers.get('set-cookie');
            if (newSetCookie) {
              recoveryHeaders.set('Set-Cookie', newSetCookie);
            }
            recoveryHeaders.set('Content-Type', 'application/json');

            return NextResponse.json(
              {
                error: 'Registration flow expired, new flow created',
                code: 'REGISTRATION_FLOW_EXPIRED_RECOVERED',
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
            console.error(`[REGISTRATION-RECOVERY] Failed to create new registration flow: ${newFlowResponse.status}`);
          }
        } catch (recoveryError) {
          console.error('[REGISTRATION-RECOVERY] Registration flow recovery failed:', recoveryError);
        }

        // Fallback to original error response if recovery fails
        return NextResponse.json(
          { error: 'Invalid registration data or expired flow', code: 'REGISTRATION_FAILED', details: errorData },
          { status: 400 }
        );
      }

      if (response.status === 410) {
        // Flow explicitly expired - create new flow
        const errorData = await response.json().catch(() => ({}));
        
        try {
          const newFlowResponse = await fetch(`${AUTH_SERVICE_URL}/v1/auth/register`, {
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
            
            const recoveryHeaders = new Headers();
            const newSetCookie = newFlowResponse.headers.get('set-cookie');
            if (newSetCookie) {
              recoveryHeaders.set('Set-Cookie', newSetCookie);
            }
            recoveryHeaders.set('Content-Type', 'application/json');

            return NextResponse.json(
              {
                error: 'Registration flow expired, new flow created',
                code: 'REGISTRATION_FLOW_EXPIRED_RECOVERED',
                recovery: {
                  new_flow_id: newFlowData.id,
                  new_flow_data: newFlowData
                },
                details: errorData
              },
              {
                status: 410,
                headers: recoveryHeaders
              }
            );
          }
        } catch (recoveryError) {
          console.error('[REGISTRATION-RECOVERY] 410 flow recovery failed:', recoveryError);
        }
        
        return NextResponse.json(
          { error: 'Registration flow expired', code: 'REGISTRATION_FLOW_EXPIRED', details: errorData },
          { status: 410 }
        );
      }

      if (response.status === 422) {
        const errorData = await response.json().catch(() => ({}));
        return NextResponse.json(
          { error: 'Registration data validation failed', code: 'VALIDATION_FAILED', details: errorData },
          { status: 422 }
        );
      }

      return NextResponse.json(
        { error: 'Registration service unavailable', code: 'SERVICE_ERROR' },
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
    console.error('[REGISTRATION-COMPLETION] Registration completion error:', {
      error: error instanceof Error ? error.message : 'Unknown error',
      stack: error instanceof Error ? error.stack : undefined,
      timestamp: new Date().toISOString(),
      authServiceUrl: AUTH_SERVICE_URL
    });

    // タイムアウトエラー処理
    if (error instanceof Error && error.name === 'AbortError') {
      return NextResponse.json(
        { error: 'Registration timeout', code: 'TIMEOUT' },
        { status: 408 }
      );
    }

    return NextResponse.json(
      { error: 'Failed to complete registration', code: 'INTERNAL_ERROR' },
      { status: 500 }
    );
  }
}