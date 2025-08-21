// middleware.ts
import { NextResponse, type NextRequest } from 'next/server'

function getOrigin(req: NextRequest): string {
  const env = process.env.NEXT_PUBLIC_APP_URL?.replace(/\/$/, '')
  if (env) return env
  const proto = req.headers.get('x-forwarded-proto') ?? 'https'
  const host  = req.headers.get('x-forwarded-host') ?? req.headers.get('host') ?? ''
  if (!host || /^0\.0\.0\.0|^127\./.test(host)) return 'https://curionoah.com'
  return `${proto}://${host}`
}

function redirectToLogin(req: NextRequest) {
  const base = getOrigin(req)
  const login = new URL('/auth/login', base)
  const returnTo = new URL(req.nextUrl.pathname + req.nextUrl.search, base)
  login.searchParams.set('return_to', returnTo.toString())
  return NextResponse.redirect(login, { headers: { 'cache-control': 'no-store' } })
}

const PUBLIC = [/^\/auth\//, /^\/_next\//, /^\/favicon\.ico$/, /^\/api\/(public|health)/]

export function middleware(req: NextRequest) {
  const p = req.nextUrl.pathname
  if (PUBLIC.some(r => r.test(p))) return NextResponse.next()
  if (p.startsWith('/auth/login') || p.startsWith('/auth/error')) return NextResponse.next()

  const hasKratos = req.cookies.has('ory_kratos_session')
  return hasKratos ? NextResponse.next() : redirectToLogin(req)
}

export const config = { matcher: ['/((?!_next/static|_next/image).*)'] }
