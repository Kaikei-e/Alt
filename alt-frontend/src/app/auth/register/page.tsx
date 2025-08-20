// app/auth/register/page.tsx
import { redirect } from 'next/navigation'
import RegisterClient from '../../register/register-client'
export const dynamic = 'force-dynamic'

export default async function RegisterPage({ searchParams }: { searchParams: Promise<Record<string, string>> }) {
  const params = await searchParams
  const flow = params.flow
  const ret  = params.return_to ?? '/' // 既定はトップ

  if (!flow) {
    const KRATOS_PUBLIC_URL = process.env.KRATOS_PUBLIC_URL
    if (!KRATOS_PUBLIC_URL || /\.cluster\.local(\b|:|\/)/i.test(KRATOS_PUBLIC_URL) || !/^https:\/\//i.test(KRATOS_PUBLIC_URL)) {
      throw new Error(`KRATOS_PUBLIC_URL must be a PUBLIC HTTPS FQDN (got: ${KRATOS_PUBLIC_URL})`)
    }
    const u = new URL(KRATOS_PUBLIC_URL + '/self-service/registration/browser')
    const app = process.env.NEXT_PUBLIC_APP_ORIGIN! // e.g. https://curionoah.com
    u.searchParams.set('return_to', new URL(ret, app).toString())
    redirect(u.toString()) // ブラウザフローはリダイレクトが仕様
  }

  return <RegisterClient returnUrl={ret} flowId={flow}/>
}
