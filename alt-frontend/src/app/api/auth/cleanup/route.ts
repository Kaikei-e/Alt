// app/api/auth/cleanup/route.ts
import { NextResponse } from 'next/server'

export const dynamic = 'force-dynamic'
export const fetchCache = 'force-no-store'

/**
 * Session cleanup endpoint for handling stale sessions
 * This endpoint clears any potentially corrupted Kratos session cookies
 * that might be causing redirect loops
 */
export async function POST() {
  try {
    const response = NextResponse.json({ 
      success: true, 
      message: 'Session cookies cleared successfully' 
    })
    
    // Clear the Kratos session cookie with domain specification
    response.cookies.delete({
      name: 'ory_kratos_session',
      domain: '.curionoah.com',
      path: '/'
    })
    
    // Also clear without domain for broader compatibility
    response.cookies.delete({
      name: 'ory_kratos_session',
      path: '/'
    })
    
    // Clear any other potential session-related cookies
    response.cookies.delete({
      name: 'ory_csrf_token',
      domain: '.curionoah.com', 
      path: '/'
    })
    response.cookies.delete({
      name: 'ory_csrf_token',
      path: '/'
    })
    
    return response
  } catch (error) {
    console.error('Session cleanup failed:', error)
    return NextResponse.json(
      { success: false, error: 'Session cleanup failed' },
      { status: 500 }
    )
  }
}

/**
 * GET endpoint for checking if cleanup is needed
 * Returns information about current session state
 */
export async function GET() {
  return NextResponse.json({
    success: true,
    message: 'Session cleanup endpoint is available',
    instructions: 'Use POST to clear session cookies'
  })
}