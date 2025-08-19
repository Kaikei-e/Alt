// app/auth/login/page.tsx
import { redirect } from 'next/navigation'
import LoginClient from '../../login/login-client'
export const dynamic = 'force-dynamic'

export default async function LoginPage({ searchParams }: { searchParams: Promise<Record<string, string>> }) {
  const params = await searchParams
  const flow = params.flow
  const ret  = params.return_to ?? '/' // 既定はルートディレクトリ

  if (!flow) {
    const u = new URL(process.env.KRATOS_PUBLIC_URL + '/self-service/login/browser')
    const app = process.env.NEXT_PUBLIC_APP_ORIGIN! // e.g. https://curionoah.com
    u.searchParams.set('return_to', new URL(ret, app).toString())
    redirect(u.toString()) // ブラウザフローはリダイレクトが仕様
  }

  return <LoginClient returnUrl={ret} flowId={flow}/>
}