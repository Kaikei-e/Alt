// alt-frontend/src/app/auth/login/page.tsx (App Router, Server Component)
import { redirect } from 'next/navigation'
import LoginForm from './LoginForm'

// SSRでは内部URL、ブラウザでは外部URLを使用
const KRATOS_SSR = process.env.KRATOS_INTERNAL_URL || 'http://kratos-public.alt-auth.svc.cluster.local:4433'
const KRATOS_BROWSER = 'https://id.curionoah.com'

async function getFlowJson(id: string) {
  try {
    const r = await fetch(`${KRATOS_SSR}/self-service/login/flows?id=${encodeURIComponent(id)}`, {
      // SSR の fetch は相手がクロスサイトでもOK。Cookieは不要（flow取得だけなら）
      headers: { Accept: 'application/json' },
      cache: 'no-store',
      signal: AbortSignal.timeout(5000), // 5秒タイムアウト
    })
    if (r.status === 410) return { expired: true }
    if (!r.ok) {
      console.error(`Failed to fetch flow ${id}: ${r.status} ${r.statusText}`)
      return { expired: true } // エラー時も期限切れとして扱い、フロー再作成に誘導
    }
    return await r.json()
  } catch (error) {
    console.error(`Error fetching flow ${id}:`, error)
    return { expired: true } // ネットワークエラー等も期限切れとして扱う
  }
}

export default async function Page({ searchParams }: { searchParams: Promise<{ flow?: string, return_to?: string }> }) {
  const params = await searchParams
  const flowId = params.flow
  const returnTo = params.return_to ?? 'https://curionoah.com/'

  if (!flowId) {
    redirect(`${KRATOS_BROWSER}/self-service/login/browser?return_to=${encodeURIComponent(returnTo)}`)
  }

  const flow = await getFlowJson(flowId!)
  if ((flow as any).expired) {
    redirect(`${KRATOS_BROWSER}/self-service/login/browser?return_to=${encodeURIComponent(returnTo)}`)
  }

  // ここで flow.ui.nodes からフォームを描画（省略）
  return <LoginForm flow={flow} />
}