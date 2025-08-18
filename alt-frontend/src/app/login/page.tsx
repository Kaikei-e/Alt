export const dynamic = 'force-dynamic'
export const revalidate = 0

import { redirect } from 'next/navigation'
import { headers } from 'next/headers'
import LoginClient from './login-client'

const KRATOS_PUBLIC   = process.env.KRATOS_PUBLIC_URL!    // 例: https://id.curionoah.com
const KRATOS_INTERNAL = process.env.KRATOS_INTERNAL_URL!  // 例: http://kratos-public.alt-auth.svc.cluster.local:4433

export default async function Page({
  searchParams,
}: { searchParams: Promise<{ flow?: string; return_to?: string; refresh?: string }> }) {
  const params  = await searchParams
  const headersList = await headers()
  const cookie  = headersList.get('cookie') ?? ''
  const backTo  = params.return_to || '/' // ← /dashboard 特別扱いナシ

  // 1) 既セッションなら /browser は叩かない（＝仕様通り）：
  //    /sessions/whoami は SSR からクラスタ内 Service に投げる
  let authed = false
  try {
    const r = await fetch(`${KRATOS_INTERNAL}/sessions/whoami`, {
      headers: { cookie }, cache: 'no-store',
    })
    authed = r.ok
  } catch (e) {
    console.error('whoami (internal) failed:', e) // フォールバックして続行（未認証扱い）
  }
  if (authed) redirect(backTo)

  // 2) セッション無し：flow が無ければブラウザフロー初期化（公開URL）
  if (!params.flow) {
    const q = params.refresh === 'true' ? '?refresh=true' : ''
    redirect(`${KRATOS_PUBLIC}/self-service/login/browser${q}`)
  }

  // 3) flow あり → JSON モードで LoginClient に渡す
  return <LoginClient flowId={params.flow!} returnUrl={backTo} />
}