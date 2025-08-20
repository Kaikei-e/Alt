// app/api/auth/validate/route.ts
import { headers } from 'next/headers'
export const dynamic = 'force-dynamic'
export const fetchCache = 'force-no-store'

export async function GET() {
  const headerStore = await headers()
  const cookie = headerStore.get('cookie') ?? ''
  
  try {
    const r = await fetch(process.env.AUTH_URL + '/v1/auth/validate', {
      headers: { 
        cookie,
        'X-Request-Id': headerStore.get('X-Request-Id') || `frontend-${Date.now()}`,
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