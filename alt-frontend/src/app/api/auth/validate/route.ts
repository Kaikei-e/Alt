// app/api/auth/validate/route.ts
import { NextRequest, NextResponse } from 'next/server'

const KRATOS_URL = process.env.KRATOS_INTERNAL_URL || process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL!

export async function GET(request: NextRequest) {
  try {
    // 受けた Cookie をそのまま Kratos /sessions/whoami に流す
    const cookieHeader = request.headers.get('Cookie')
    
    const whoamiResponse = await fetch(`${KRATOS_URL}/sessions/whoami`, {
      method: 'GET',
      headers: {
        'Cookie': cookieHeader || '',
        'Accept': 'application/json',
      },
      cache: 'no-store',
    })

    if (!whoamiResponse.ok) {
      // 失敗時は401を返す
      return NextResponse.json(
        { error: 'Unauthorized', status: whoamiResponse.status },
        { status: 401 }
      )
    }

    const sessionData = await whoamiResponse.json()
    return NextResponse.json(sessionData, { status: 200 })

  } catch (error) {
    console.error('whoami validation failed:', error)
    return NextResponse.json(
      { error: 'Internal Server Error' },
      { status: 500 }
    )
  }
}