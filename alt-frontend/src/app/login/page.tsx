export const dynamic = 'force-dynamic'
export const revalidate = 0

import { redirect } from 'next/navigation'
import { headers } from 'next/headers'
import LoginClient from './login-client'

const KRATOS = 'https://id.curionoah.com'

export default async function Page({
  searchParams,
}: { searchParams: Promise<{ flow?: string; return_to?: string; refresh?: string }> }) {
  const params = await searchParams
  const headersList = await headers()
  const cookie = headersList.get('cookie') ?? ''

  // 1) 既セッションなら /browser は叩かずダッシュボードへ
  const who = await fetch(`${KRATOS}/sessions/whoami`, {
    headers: { cookie },
    cache: 'no-store',
  })
  if (who.ok) redirect(params.return_to || '/dashboard')

  // 2) セッション無し：flowが無ければブラウザフローを初期化
  if (!params.flow) {
    const q = params.refresh === 'true' ? '?refresh=true' : ''
    redirect(`${KRATOS}/self-service/login/browser${q}`)
  }

  // 3) flow あり → クライアントに任せる（JSONモードのみ）
  return <LoginClient flowId={params.flow!} returnUrl={params.return_to || '/dashboard'} />
}