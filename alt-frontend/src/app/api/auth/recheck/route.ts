// app/api/auth/recheck/route.ts
import { headers } from 'next/headers'
export const dynamic = 'force-dynamic'
export const fetchCache = 'force-no-store'

export async function GET() {
  try {
    const cookie = (await headers()).get('cookie') ?? ''
    
    // First try auth-service validation
    const authServiceUrl = process.env.AUTH_URL || process.env.AUTH_SERVICE_URL
    if (authServiceUrl) {
      try {
        const authResponse = await fetch(authServiceUrl + '/v1/auth/validate', {
          headers: { cookie },
          cache: 'no-store',
          signal: AbortSignal.timeout(3000)
        })
        
        if (authResponse.status === 200) {
          return new Response(JSON.stringify({ status: 'valid', source: 'auth-service' }), { 
            status: 200,
            headers: { 'Content-Type': 'application/json' }
          })
        }
      } catch (authError) {
        console.warn('Auth service recheck failed:', authError)
      }
    }
    
    // Fallback to Kratos direct check
    const kratosUrl = process.env.KRATOS_INTERNAL_URL || process.env.KRATOS_PUBLIC_URL
    if (kratosUrl) {
      try {
        const kratosResponse = await fetch(kratosUrl + '/sessions/whoami', {
          headers: { cookie },
          cache: 'no-store',
          signal: AbortSignal.timeout(3000)
        })
        
        if (kratosResponse.status === 200) {
          return new Response(JSON.stringify({ status: 'valid', source: 'kratos-direct' }), { 
            status: 200,
            headers: { 'Content-Type': 'application/json' }
          })
        }
      } catch (kratosError) {
        console.warn('Kratos direct recheck failed:', kratosError)
      }
    }
    
    // All checks failed
    return new Response(JSON.stringify({ status: 'invalid' }), { 
      status: 401,
      headers: { 'Content-Type': 'application/json' }
    })
    
  } catch (error) {
    console.error('Recheck endpoint error:', error)
    return new Response(JSON.stringify({ status: 'error', message: 'Recheck failed' }), { 
      status: 503,
      headers: { 'Content-Type': 'application/json' }
    })
  }
}