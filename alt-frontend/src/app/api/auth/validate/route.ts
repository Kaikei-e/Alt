// app/api/auth/validate/route.ts
import { headers } from 'next/headers'
export const dynamic = 'force-dynamic'
export const fetchCache = 'force-no-store'

export async function GET() {
  const headerStore = headers()
  const cookie = (await headerStore).get('cookie') ?? ''
  
  const url = `${process.env.AUTH_SERVICE_URL ?? 'http://auth-service.alt-auth.svc.cluster.local:8080'}/v1/auth/validate`

  try {
    const r = await fetch(url, {
      headers: { 
        'Cookie': cookie,
        'X-Request-Source': 'frontend-ssr',
        'X-Request-Id': (await headerStore).get('X-Request-Id') || `frontend-${Date.now()}`,
      },
      cache: 'no-store',
    })
    
    if (r.status === 200) {
      const data = await r.json()
      return Response.json(data, { 
        status: 200,
        headers: {
          'Content-Type': 'application/json; charset=utf-8',
          'Cache-Control': 'no-store',
        }
      })
    }
    
    if (r.status === 401) {
      const errorData = await r.json().catch(() => ({ message: 'unauthorized' }))
      return Response.json(errorData, { 
        status: 401,
        headers: {
          'Content-Type': 'application/json; charset=utf-8',
        }
      })
    }
    
    // Other status codes
    return Response.json(
      { message: 'auth service unavailable' }, 
      { status: 502 }
    )
  } catch (error) {
    return Response.json(
      { message: 'auth service unavailable' }, 
      { status: 502 }
    )
  }
}