// src/app/auth/login/route.ts
import { redirect } from 'next/navigation'

export async function GET(req: Request) {
  const app = process.env.NEXT_PUBLIC_APP_URL || 'https://curionoah.com'
  const idp = process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL || 'https://id.curionoah.com'
  const url = new URL(req.url)
  const rt = url.searchParams.get('return_to') || `${app}/`
  redirect(`${idp}/self-service/login/browser?return_to=${encodeURIComponent(rt)}`)
}