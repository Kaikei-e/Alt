// app/auth/login/page.tsx (Server Component)
import { redirect } from 'next/navigation'
import { headers } from 'next/headers'
import LoginClient from '@/app/login/login-client'
import { KRATOS_PUBLIC_URL } from '@/lib/env.public'

const KRATOS = KRATOS_PUBLIC_URL  // e.g. https://id.curionoah.com
const APP = process.env.NEXT_PUBLIC_APP_URL ?? process.env.NEXT_PUBLIC_APP_ORIGIN ?? 'https://curionoah.com'

export default async function LoginPage({
  searchParams,
}: { searchParams: Promise<{ flow?: string; return_to?: string }> }) {
  const params = await searchParams
  const flow = params.flow
  
  // return_to の厳格化：保護ページの絶対URLを設定
  let returnTo = params.return_to ?? `${APP}/home`
  
  // 🔧 修正: 無限ループを防ぐためのより厳密なチェック
  if (returnTo.includes('/auth/login') || returnTo.includes('/auth/')) {
    returnTo = `${APP}/home`
  }
  
  // 相対URLの場合は絶対URLに変換
  if (!returnTo.startsWith('http')) {
    returnTo = new URL(returnTo, APP).toString()
  }

  // flow が無ければブラウザフロー開始
  if (!flow) {
    // TODO.md: Kratos whoami直接使用（auth-service経由ではなく）
    // Before redirecting to Kratos, check if already authenticated
    // This prevents redirect loops when user has valid session
    try {
      const headersList = await headers()
      const cookie = headersList.get('cookie') ?? ''

      // TODO.md: Kratosの/sessions/whoamiを直接呼び出し（Option A: Kratosのみ）
      const kratosInternalUrl = process.env.KRATOS_INTERNAL_URL ?? `${KRATOS}`
      const validateRes = await fetch(`${kratosInternalUrl}/sessions/whoami`, {
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
    // ここで return_to を必ず付ける（既ログインなら即保護ページに戻る）
    u.searchParams.set('return_to', returnTo)
    redirect(u.toString()) // try/catchで包まない（redirect は throw）
  }

  // flow がある場合のみ UI を描画（クライアントがフォーム送信を担当）
  return <LoginClient flowId={flow} returnUrl={returnTo} />
}
