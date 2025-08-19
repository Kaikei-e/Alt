// app/auth/login/page.tsx (Server Component)
import { redirect } from 'next/navigation'
import { headers } from 'next/headers'
import LoginClient from '@/app/login/login-client'

const KRATOS = process.env.KRATOS_PUBLIC_URL!          // e.g. https://id.curionoah.com
const APP = process.env.NEXT_PUBLIC_APP_ORIGIN!     // e.g. https://curionoah.com

export default async function LoginPage({
  searchParams,
}: { searchParams: Promise<{ flow?: string; return_to?: string }> }) {
  const params = await searchParams
  const flow = params.flow
  const returnTo = params.return_to ?? '/'        // 既定はルート（要件通り）

  // flow が無ければブラウザフロー開始
  if (!flow) {
    // Before redirecting to Kratos, check if already authenticated
    // This prevents redirect loops when user has valid session
    try {
      const headersList = await headers()
      const cookie = headersList.get('cookie') ?? ''

      const validateRes = await fetch(`${process.env.AUTH_URL}/v1/auth/validate`, {
        headers: { cookie },
        cache: 'no-store',
      })

      // If already authenticated, redirect to returnTo instead of starting new flow
      if (validateRes.ok) {
        redirect(returnTo)
      }
    } catch (error) {
      // If validation fails, proceed with normal login flow
      console.log('Auth validation failed, proceeding with login flow:', error)
    }

    const u = new URL('/self-service/login/browser', KRATOS)
    // ここで return_to を必ず付ける（既ログインなら即 / に戻る）
    u.searchParams.set('return_to', new URL(returnTo, APP).toString())
    redirect(u.toString()) // try/catchで包まない（redirect は throw）
  }

  // flow がある場合のみ UI を描画（クライアントがフォーム送信を担当）
  return <LoginClient flowId={flow} returnUrl={returnTo} />
}